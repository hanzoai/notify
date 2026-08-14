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
//
// `body` is the RAW decoded JSON, never `emailRequest`. Decoding into the
// struct that encoded it makes every field name agree with itself by
// construction, so the one thing a wire format can get wrong is the one
// thing such a test cannot see. This suite was green throughout the life
// of a body Twilio does not read.
type captured struct {
	method      string
	path        string
	contentType string
	authUser    string
	authPass    string
	authOK      bool
	body        map[string]any
}

// at walks the decoded body by key path, e.g. at(t, got.body, "content",
// "html"), failing with the path it could not follow — which is what a
// renamed field looks like from out here.
func at(t *testing.T, body map[string]any, path ...string) any {
	t.Helper()
	var cur any = body
	for i, key := range path {
		obj, ok := cur.(map[string]any)
		require.Truef(t, ok, "%v is not an object", path[:i])
		cur, ok = obj[key]
		require.Truef(t, ok, "request body has no %v", path[:i+1])
	}
	return cur
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
	assert.Equal(t, "no-reply@send.hanzo.ai", at(t, got.body, "from", "address"))
	assert.Equal(t, "Hanzo", at(t, got.body, "from", "name"))
	assert.Equal(t, "Welcome", at(t, got.body, "content", "subject"))

	to, ok := at(t, got.body, "to").([]any)
	require.True(t, ok, "to must be an array")
	require.Len(t, to, 2)
	assert.Equal(t, "alice@example.com", to[0].(map[string]any)["address"])
	assert.Equal(t, "bob@example.com", to[1].(map[string]any)["address"])
	// Default body type is HTML.
	assert.Equal(t, "<b>hello</b>", at(t, got.body, "content", "html"))
	assert.NotContains(t, at(t, got.body, "content"), "text")

	// Nothing rides at the top level but the envelope. A `subject` or an
	// `html` here is the flat shape coming back.
	for _, key := range []string{"subject", "html", "text", "email"} {
		assert.NotContains(t, got.body, key)
	}
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

	assert.Equal(t, "plain body", at(t, got.body, "content", "text"))
	// Twilio requires a non-empty html part; plain text rides as an
	// HTML-escaped <pre> copy.
	assert.Equal(t, "<pre>plain body</pre>", at(t, got.body, "content", "html"))
	// Empty sender name falls back to the sender address.
	assert.Equal(t, "no-reply@send.hanzo.ai", at(t, got.body, "from", "name"))
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

// TestService_SendWithHeaders_CarriesRFC8058 proves the RFC 8058
// List-Unsubscribe / List-Unsubscribe-Post pair reaches the Twilio Emails
// request body via the headers map — the marketing chain's RawSender path.
func TestService_SendWithHeaders_CarriesRFC8058(t *testing.T) {
	t.Parallel()

	srv, got := newFakeTwilio(t, http.StatusOK)

	s := New("AC_fake_sid", "fake_token", "no-reply@send.hanzo.ai", "Hanzo")
	s.baseURL = srv.URL + "/v1/Emails"
	s.client = srv.Client()
	s.AddReceivers("alice@example.com")

	headers := map[string]string{
		"List-Unsubscribe":      "<https://hanzo.ai/u/abc>, <mailto:unsub@hanzo.ai>",
		"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
	}
	require.NoError(t, s.SendWithHeaders(context.Background(), "Promo", "<b>deal</b>", headers))

	sent, ok := at(t, got.body, "content", "headers").(map[string]any)
	require.True(t, ok, "headers must be an object")
	require.Len(t, sent, 2)
	assert.Equal(t, "<https://hanzo.ai/u/abc>, <mailto:unsub@hanzo.ai>", sent["List-Unsubscribe"])
	assert.Equal(t, "List-Unsubscribe=One-Click", sent["List-Unsubscribe-Post"])
}

// TestService_Send_OmitsHeaders proves the plain Send path produces no
// headers field — byte-identical to the pre-RawSender body.
func TestService_Send_OmitsHeaders(t *testing.T) {
	t.Parallel()

	srv, got := newFakeTwilio(t, http.StatusOK)

	s := New("AC_fake_sid", "fake_token", "no-reply@send.hanzo.ai", "Hanzo")
	s.baseURL = srv.URL + "/v1/Emails"
	s.client = srv.Client()
	s.AddReceivers("alice@example.com")

	require.NoError(t, s.Send(context.Background(), "Subject", "body"))
	assert.NotContains(t, at(t, got.body, "content"), "headers")
}
