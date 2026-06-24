package amazonses

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestAmazonSES_Send(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		receivers     []string
		subject       string
		message       string
		mockSetup     func(*mocksesClient)
		expectedError string
	}{
		{
			name:      "Successful send",
			receivers: []string{"test@example.com"},
			subject:   "Test Subject",
			message:   "Test Message",
			mockSetup: func(m *mocksesClient) {
				m.On("SendEmail", mock.Anything, mock.AnythingOfType("*ses.SendEmailInput")).
					Return(&ses.SendEmailOutput{}, nil)
			},
			expectedError: "",
		},
		{
			name:      "SES client error",
			receivers: []string{"test@example.com"},
			subject:   "Test Subject",
			message:   "Test Message",
			mockSetup: func(m *mocksesClient) {
				m.On("SendEmail", mock.Anything, mock.AnythingOfType("*ses.SendEmailInput")).
					Return(nil, errors.New("SES error"))
			},
			expectedError: "send mail using Amazon SES service: SES error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := new(mocksesClient)
			tt.mockSetup(mockClient)

			s := &AmazonSES{
				client:            mockClient,
				senderAddress:     aws.String("sender@example.com"),
				receiverAddresses: tt.receivers,
			}

			err := s.Send(context.Background(), tt.subject, tt.message)

			if tt.expectedError != "" {
				require.EqualError(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestAmazonSES_SendRaw(t *testing.T) {
	t.Parallel()

	t.Run("raw MIME carries List-Unsubscribe headers and body", func(t *testing.T) {
		t.Parallel()

		var captured *ses.SendRawEmailInput
		mockClient := new(mocksesClient)
		mockClient.On("SendRawEmail", mock.Anything, mock.AnythingOfType("*ses.SendRawEmailInput")).
			Run(func(args mock.Arguments) {
				captured = args.Get(1).(*ses.SendRawEmailInput)
			}).
			Return(&ses.SendRawEmailOutput{}, nil)

		s := &AmazonSES{
			client:            mockClient,
			senderAddress:     aws.String("no-reply@send.hanzo.ai"),
			receiverAddresses: []string{"user@example.com"},
		}

		headers := map[string]string{
			"List-Unsubscribe":      "<https://notify.hanzo.ai/v1/notify/unsubscribe/tok>, <mailto:unsub@hanzo.ai>",
			"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
		}
		// The body contains an '=' which quoted-printable MUST escape to =3D —
		// proving the body really is QP-encoded, not passed through raw.
		err := s.SendRaw(context.Background(), "Weekly digest", "<p>a=b & welcome</p>", headers)
		require.NoError(t, err)
		require.NotNil(t, captured)

		raw := string(captured.RawMessage.Data)
		require.Contains(t, raw, "From: no-reply@send.hanzo.ai")
		require.Contains(t, raw, "To: user@example.com")
		require.Contains(t, raw, "Subject: Weekly digest")
		require.Contains(t, raw, "List-Unsubscribe: <https://notify.hanzo.ai/v1/notify/unsubscribe/tok>, <mailto:unsub@hanzo.ai>")
		require.Contains(t, raw, "List-Unsubscribe-Post: List-Unsubscribe=One-Click")
		require.Contains(t, raw, "Content-Type: text/html; charset=UTF-8")
		// quoted-printable escapes '=' as =3D (deterministic body encoding);
		// '&' and angle brackets are QP-safe literals and stay as-is.
		require.Contains(t, raw, "a=3Db & welcome")
		// destination is threaded to the raw-email envelope, not just the MIME To.
		require.Equal(t, []string{"user@example.com"}, captured.Destinations)
		// headers precede the body separator.
		require.True(t, strings.Index(raw, "List-Unsubscribe:") < strings.Index(raw, "\r\n\r\n"))

		mockClient.AssertExpectations(t)
	})

	t.Run("propagates SES error", func(t *testing.T) {
		t.Parallel()

		mockClient := new(mocksesClient)
		mockClient.On("SendRawEmail", mock.Anything, mock.AnythingOfType("*ses.SendRawEmailInput")).
			Return(nil, errors.New("SES raw error"))

		s := &AmazonSES{
			client:            mockClient,
			senderAddress:     aws.String("no-reply@send.hanzo.ai"),
			receiverAddresses: []string{"user@example.com"},
		}
		err := s.SendRaw(context.Background(), "S", "B", nil)
		require.EqualError(t, err, "send raw mail using Amazon SES service: SES raw error")
		mockClient.AssertExpectations(t)
	})
}
