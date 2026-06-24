package amazonses

import (
	"context"
	"fmt"

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

// SendRaw sends a pre-built raw MIME message via the SES SendRawEmail API.
// Unlike Send (which uses the structured SendEmail API and cannot carry
// arbitrary headers), the raw path lets the caller inject custom headers —
// e.g. RFC 8058 List-Unsubscribe / List-Unsubscribe-Post for the marketing
// chain. The message must already be a complete, standards-compliant MIME
// document (headers + body); the SDK base64-encodes it for the wire.
//
// Source and Destinations are passed explicitly so the envelope sender and
// recipients are authoritative even when they differ from the MIME headers.
func (a AmazonSES) SendRaw(ctx context.Context, raw []byte) error {
	input := &ses.SendRawEmailInput{
		RawMessage: &types.RawMessage{
			Data: raw,
		},
		Source:       a.senderAddress,
		Destinations: a.receiverAddresses,
	}

	_, err := a.client.SendRawEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("send raw mail using Amazon SES service: %w", err)
	}

	return nil
}
