package mail

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMail_newEmailHtml(t *testing.T) {
	t.Parallel()

	text := "test"
	m := New("foo", "server")
	email := m.newEmail("test", text)

	assert.False(t, m.usePlainText)
	assert.Equal(t, []byte(nil), email.Text)
	assert.Equal(t, []byte(text), email.HTML)
}

func TestMail_newEmailText(t *testing.T) {
	t.Parallel()

	text := "test"
	m := New("foo", "server")
	m.BodyFormat(PlainText)
	email := m.newEmail("test", text)

	assert.True(t, m.usePlainText)
	assert.Equal(t, []byte(text), email.Text)
	assert.Equal(t, []byte(nil), email.HTML)
}

// An HTML body carries its plain-text part beside it; a plain-text body is that
// part already, and the alternative adds nothing to it.
func TestMail_newEmailAlternative(t *testing.T) {
	t.Parallel()

	m := New("foo", "server")
	m.Alternative("Sign in: https://example.com")
	email := m.newEmail("test", "<p>Sign in</p>")
	assert.Equal(t, []byte("<p>Sign in</p>"), email.HTML)
	assert.Equal(t, []byte("Sign in: https://example.com"), email.Text)

	plain := New("foo", "server")
	plain.BodyFormat(PlainText)
	plain.Alternative("ignored")
	email = plain.newEmail("test", "plain body")
	assert.Equal(t, []byte("plain body"), email.Text)
	assert.Equal(t, []byte(nil), email.HTML)
}

func TestMail_AddReceivers(t *testing.T) {
	t.Parallel()

	m := New("foo", "server")
	m.AddReceivers("test")

	assert.Len(t, m.receiverAddresses, 1)
	assert.Equal(t, "test", m.receiverAddresses[0])
}

func TestMail_AuthenticateSMTP(t *testing.T) {
	t.Parallel()

	m := New("foo", "server")
	assert.Nil(t, m.smtpAuth)

	m.AuthenticateSMTP("test", "test", "test", "test")
	assert.NotNil(t, m.smtpAuth)
}
