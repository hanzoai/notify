package amazonses

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
)

//go:generate mockery --name=sesClient --output=. --case=underscore --inpackage
type sesClient interface {
	SendEmail(
		ctx context.Context,
		params *ses.SendEmailInput,
		optFns ...func(options *ses.Options),
	) (*ses.SendEmailOutput, error)
	SendRawEmail(
		ctx context.Context,
		params *ses.SendRawEmailInput,
		optFns ...func(options *ses.Options),
	) (*ses.SendRawEmailOutput, error)
}

// Compile-time check to ensure that ses.Client implements the sesClient interface.
var _ sesClient = new(ses.Client)

// AmazonSES struct holds necessary data to communicate with the Amazon Simple Email Service API.
type AmazonSES struct {
	client            sesClient
	senderAddress     *string
	receiverAddresses []string
}

// New returns a new instance of a AmazonSES notification service.
// You will need an Amazon Simple Email Service API access key and secret.
// See https://aws.github.io/aws-sdk-go-v2/docs/getting-started/
func New(accessKeyID, secretKey, region, senderAddress string) (*AmazonSES, error) {
	credProvider := credentials.NewStaticCredentialsProvider(accessKeyID, secretKey, "")

	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithCredentialsProvider(credProvider),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, err
	}

	return &AmazonSES{
		client:            ses.NewFromConfig(cfg),
		senderAddress:     aws.String(senderAddress),
		receiverAddresses: []string{},
	}, nil
}

// AddReceivers takes email addresses and adds them to the internal address list. The Send method will send
// a given message to all those addresses.
func (a *AmazonSES) AddReceivers(addresses ...string) {
	a.receiverAddresses = append(a.receiverAddresses, addresses...)
}

// Send takes a message subject and a message body and sends them to all previously set chats. Message body supports
// html as markup language.
func (a AmazonSES) Send(ctx context.Context, subject, message string) error {
	input := &ses.SendEmailInput{
		Source: a.senderAddress,
		Destination: &types.Destination{
			ToAddresses: a.receiverAddresses,
		},
		Message: &types.Message{
			Body: &types.Body{
				Html: &types.Content{
					Data: aws.String(message),
				},
				// Text: &types.Content{
				//     Data:    aws.String(message),
				// },
			},
			Subject: &types.Content{
				Data: aws.String(subject),
			},
		},
	}

	_, err := a.client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("send mail using Amazon SES service: %w", err)
	}

	return nil
}

// SendRaw sends an HTML message with caller-supplied extra MIME headers
// (e.g. RFC 8058 List-Unsubscribe / List-Unsubscribe-Post) via the SES
// SendRawEmail API. The structured SendEmail call cannot carry custom
// headers, so the raw-MIME path is the only way for those headers to ride
// along. Header keys are emitted in sorted order for deterministic output.
func (a AmazonSES) SendRaw(ctx context.Context, subject, message string, extraHeaders map[string]string) error {
	if a.senderAddress == nil {
		return fmt.Errorf("send raw mail using Amazon SES service: sender address not set")
	}

	var buf bytes.Buffer
	buf.WriteString("From: " + aws.ToString(a.senderAddress) + "\r\n")
	if len(a.receiverAddresses) > 0 {
		buf.WriteString("To: " + strings.Join(a.receiverAddresses, ", ") + "\r\n")
	}
	buf.WriteString("Subject: " + mimeEncodeHeader(subject) + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")

	keys := make([]string, 0, len(extraHeaders))
	for k := range extraHeaders {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf.WriteString(k + ": " + extraHeaders[k] + "\r\n")
	}

	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	if err := writeQuotedPrintable(&buf, message); err != nil {
		return fmt.Errorf("send raw mail using Amazon SES service: encode body: %w", err)
	}

	input := &ses.SendRawEmailInput{
		Source:       a.senderAddress,
		Destinations: a.receiverAddresses,
		RawMessage:   &types.RawMessage{Data: buf.Bytes()},
	}

	if _, err := a.client.SendRawEmail(ctx, input); err != nil {
		return fmt.Errorf("send raw mail using Amazon SES service: %w", err)
	}

	return nil
}

// mimeEncodeHeader RFC 2047-encodes a header value when it contains
// non-ASCII bytes; pure-ASCII values pass through unchanged.
func mimeEncodeHeader(v string) string {
	for i := 0; i < len(v); i++ {
		if v[i] > 127 {
			return mime.QEncoding.Encode("UTF-8", v)
		}
	}
	return v
}

// writeQuotedPrintable encodes body as quoted-printable into w.
func writeQuotedPrintable(w *bytes.Buffer, body string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := io.WriteString(qp, body); err != nil {
		return err
	}
	return qp.Close()
}
