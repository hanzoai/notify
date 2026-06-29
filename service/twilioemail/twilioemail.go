package twilioemail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
)

// defaultBaseURL is Twilio's native Email API endpoint. Email sends POST
// here with HTTP Basic auth using the SAME Twilio account credentials
// (ACCOUNT_SID:AUTH_TOKEN) that the SMS provider uses — no SendGrid key.
//
// See https://www.twilio.com/docs (Programmable Messaging / Email).
const defaultBaseURL = "https://comms.twilio.com/v1/Emails"

// BodyType selects the format of the message body. Default is HTML to
// match the sendgrid provider's default (notify treats the email body as
// HTML unless told otherwise — see routes.SendInput.IsHTML).
type BodyType int

const (
	// HTML is used to specify that the body is HTML. This is the default.
	HTML BodyType = iota
	// PlainText is used to specify that the body is plain text.
	PlainText
)

// Service is the Twilio native Email notification provider. It satisfies
// notify.Notifier (Send) and mirrors the sendgrid.SendGrid surface
// (AddReceivers + BodyFormat) so the tenant chain can wrap it with the
// same adapter shape.
type Service struct {
	accountSID    string
	authToken     string
	senderAddress string
	senderName    string

	usePlainText      bool
	receiverAddresses []string

	// baseURL is the Twilio Emails endpoint. Overridable in tests so the
	// request can be pointed at an httptest server (the same seam the
	// service/http provider uses for its client). Production callers never
	// set it — it defaults to defaultBaseURL.
	baseURL string
	// client is the HTTP client used for the POST. nil → http.DefaultClient.
	client *http.Client
}

// emailRequest is the JSON body POSTed to POST /v1/Emails.
//
// Schema verified against the live comms.twilio.com/v1/Emails API
// (2026-06): a recipient/sender is {address,name}; subject + body live
// under a nested content object. The API returns 202 with
// {"operationId","operationLocation"} on accept.
type emailRequest struct {
	From    emailAddress   `json:"from"`
	To      []emailAddress `json:"to"`
	Content emailContent   `json:"content"`
}

// emailAddress is the {address,name} pair used for both sender and
// recipients. name is omitted when empty.
type emailAddress struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// emailContent is the nested content object. The API REQUIRES a non-empty
// html part; text is an optional additional plain part; headers carries
// caller-supplied MIME headers (the RFC 8058 List-Unsubscribe /
// List-Unsubscribe-Post pair) and nests here, not at the top level.
type emailContent struct {
	Subject string            `json:"subject"`
	HTML    string            `json:"html"`
	Text    string            `json:"text,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// New returns a new Twilio Email provider. accountSID + authToken are the
// shared Twilio account credentials (the same pair the SMS provider
// uses); senderAddress is the verified from-email on the Twilio sender
// domain (e.g. no-reply@send.hanzo.ai); senderName is the optional
// display name (falls back to senderAddress when empty).
func New(accountSID, authToken, senderAddress, senderName string) *Service {
	if strings.TrimSpace(senderName) == "" {
		senderName = senderAddress
	}
	return &Service{
		accountSID:        accountSID,
		authToken:         authToken,
		senderAddress:     senderAddress,
		senderName:        senderName,
		receiverAddresses: []string{},
		baseURL:           defaultBaseURL,
		client:            http.DefaultClient,
	}
}

// AddReceivers appends recipient email addresses to the internal list.
// Send delivers a given message to all of them.
func (s *Service) AddReceivers(addresses ...string) {
	s.receiverAddresses = append(s.receiverAddresses, addresses...)
}

// BodyFormat selects the body format. Default is HTML.
func (s *Service) BodyFormat(format BodyType) {
	switch format {
	case PlainText:
		s.usePlainText = true
	case HTML:
		s.usePlainText = false
	default:
		s.usePlainText = false
	}
}

// Send POSTs the message to Twilio's Emails endpoint, addressed to every
// previously-added recipient. subject + message become the email subject
// and body; the body is sent as HTML unless BodyFormat(PlainText) was set.
func (s *Service) Send(ctx context.Context, subject, message string) error {
	return s.send(ctx, subject, message, nil)
}

// SendWithHeaders is Send plus a custom MIME-header map carried in the
// Twilio Emails request body — the RFC 8058 List-Unsubscribe /
// List-Unsubscribe-Post pair the marketing path needs. A nil/empty map
// behaves exactly like Send.
func (s *Service) SendWithHeaders(ctx context.Context, subject, message string, headers map[string]string) error {
	return s.send(ctx, subject, message, headers)
}

// send is the single POST path shared by Send and SendWithHeaders.
func (s *Service) send(ctx context.Context, subject, message string, headers map[string]string) error {
	if len(s.receiverAddresses) == 0 {
		return nil
	}

	content := emailContent{Subject: subject}
	if s.usePlainText {
		// Twilio's Email API requires a non-empty html part, so carry the
		// plain text in both: text/plain plus an HTML-escaped copy.
		content.Text = message
		content.HTML = "<pre>" + html.EscapeString(message) + "</pre>"
	} else {
		content.HTML = message
	}
	if len(headers) > 0 {
		content.Headers = headers
	}
	reqBody := emailRequest{
		From:    emailAddress{Address: s.senderAddress, Name: s.senderName},
		To:      make([]emailAddress, 0, len(s.receiverAddresses)),
		Content: content,
	}
	for _, addr := range s.receiverAddresses {
		reqBody.To = append(reqBody.To, emailAddress{Address: addr})
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal email request: %w", err)
	}

	url := s.baseURL
	if url == "" {
		url = defaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build email request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Same account credentials as the Twilio SMS provider.
	req.SetBasicAuth(s.accountSID, s.authToken)

	client := s.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("twilio email endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
