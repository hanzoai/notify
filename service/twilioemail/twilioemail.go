package twilioemail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
// SCHEMA ASSUMPTION: Twilio's native Emails API is JSON over HTTP Basic,
// but the exact field names of the request body are not pinned in this
// codebase. This struct implements the most standard transactional-email
// shape (from / to / subject / text+html bodies). It is isolated here so
// that if Twilio's published schema uses different field names it is a
// one-struct, one-line-per-field fix with NO change to Service/Send.
type emailRequest struct {
	From    emailAddress   `json:"from"`
	To      []emailAddress `json:"to"`
	Subject string         `json:"subject"`
	// Exactly one of Text/HTML is populated per send (HTML by default).
	Text string `json:"text,omitempty"`
	HTML string `json:"html,omitempty"`
	// Headers carries caller-supplied MIME headers (e.g. the RFC 8058
	// List-Unsubscribe / List-Unsubscribe-Post pair). Omitted when empty
	// so the structured Send path produces an identical body to before.
	Headers map[string]string `json:"headers,omitempty"`
}

// emailAddress is the {email,name} pair used for both sender and
// recipients. name is omitted when empty.
type emailAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
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

	reqBody := emailRequest{
		From:    emailAddress{Email: s.senderAddress, Name: s.senderName},
		To:      make([]emailAddress, 0, len(s.receiverAddresses)),
		Subject: subject,
	}
	for _, addr := range s.receiverAddresses {
		reqBody.To = append(reqBody.To, emailAddress{Email: addr})
	}
	if s.usePlainText {
		reqBody.Text = message
	} else {
		reqBody.HTML = message
	}
	if len(headers) > 0 {
		reqBody.Headers = headers
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
