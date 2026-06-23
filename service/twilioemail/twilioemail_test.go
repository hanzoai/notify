package twilioemail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captured holds what the fake Twilio Emails endpoint received so the
// test can assert URL, auth, and JSON body without a real send.
type captured struct {
	method      string
	path        string
	contentType string
	authUser    string
	authPass    string
	authOK      bool
	body        emailRequest
}

// newFakeTwilio returns an httptest server standing in for
// comms.twilio.com plus a pointer the handler fills on each request.
func newFakeTwilio(t *testing.T, status int) (*httptest.Server, *captured) {
	t.Helper()
	got := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.contentType = r.Header.Get("Content-Type")
		got.authUser, got.authPass, got.authOK = r.BasicAuth()

		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &got.body))

		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestService_Send_BuildsCorrectRequest(t *testing.T) {
	t.Parallel()

	srv, got := newFakeTwilio(t, http.StatusCreated)

	s := New("AC_fake_sid", "fake_token", "no-reply@send.hanzo.ai", "Hanzo")
	s.baseURL = srv.URL + "/v1/Emails"
	s.client = srv.Client()
	s.AddReceivers("alice@example.com", "bob@example.com")

	err := s.Send(context.Background(), "Welcome", "<b>hello</b>")
	require.NoError(t, err)

	// URL + method.
	assert.Equal(t, http.MethodPost, got.method)
	assert.Equal(t, "/v1/Emails", got.path)
	assert.Equal(t, "application/json", got.contentType)

	// HTTP Basic with the shared Twilio account creds.
	assert.True(t, got.authOK, "request must carry HTTP Basic auth")
	assert.Equal(t, "AC_fake_sid", got.authUser)
	assert.Equal(t, "fake_token", got.authPass)

	// JSON body shape.
	assert.Equal(t, "no-reply@send.hanzo.ai", got.body.From.Email)
	assert.Equal(t, "Hanzo", got.body.From.Name)
	assert.Equal(t, "Welcome", got.body.Subject)
	require.Len(t, got.body.To, 2)
	assert.Equal(t, "alice@example.com", got.body.To[0].Email)
	assert.Equal(t, "bob@example.com", got.body.To[1].Email)
	// Default body type is HTML.
	assert.Equal(t, "<b>hello</b>", got.body.HTML)
	assert.Empty(t, got.body.Text)
}

func TestService_Send_PlainText(t *testing.T) {
	t.Parallel()

	srv, got := newFakeTwilio(t, http.StatusOK)

	s := New("AC_fake_sid", "fake_token", "no-reply@send.hanzo.ai", "")
	s.baseURL = srv.URL + "/v1/Emails"
	s.client = srv.Client()
	s.BodyFormat(PlainText)
	s.AddReceivers("alice@example.com")

	require.NoError(t, s.Send(context.Background(), "Subject", "plain body"))

	assert.Equal(t, "plain body", got.body.Text)
	assert.Empty(t, got.body.HTML)
	// Empty sender name falls back to the sender address.
	assert.Equal(t, "no-reply@send.hanzo.ai", got.body.From.Name)
}

func TestService_Send_Non2xxIsError(t *testing.T) {
	t.Parallel()

	srv, _ := newFakeTwilio(t, http.StatusUnauthorized)

	s := New("AC_fake_sid", "bad_token", "no-reply@send.hanzo.ai", "Hanzo")
	s.baseURL = srv.URL + "/v1/Emails"
	s.client = srv.Client()
	s.AddReceivers("alice@example.com")

	err := s.Send(context.Background(), "Subject", "body")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestService_Send_NoReceiversNoop(t *testing.T) {
	t.Parallel()

	// No receivers → no HTTP call, no error. baseURL points nowhere so a
	// stray request would fail the test loudly.
	s := New("AC_fake_sid", "fake_token", "no-reply@send.hanzo.ai", "Hanzo")
	s.baseURL = "http://127.0.0.1:0/should-not-be-called"
	require.NoError(t, s.Send(context.Background(), "Subject", "body"))
}
