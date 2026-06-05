package storagelock

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckEnvCleanReturnsNil(t *testing.T) {
	if err := CheckEnv(func(string) string { return "" }); err != nil {
		t.Fatalf("expected nil for empty env, got %v", err)
	}
}

func TestCheckEnvNilLookup(t *testing.T) {
	if err := CheckEnv(nil); err != nil {
		t.Fatalf("expected nil for nil lookup, got %v", err)
	}
}

func TestCheckEnvDatabaseURLPostgresRejected(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL": "postgres://hanzo:secret@postgres.hanzo.svc:5432/notify?sslmode=disable",
	}
	err := CheckEnv(mapLookup(env))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPostgresForbidden) {
		// Direct unwrapping is intentional — we want the lockdown report
		// to be matchable via errors.Is for ops alerting.
		if !strings.Contains(err.Error(), ErrPostgresForbidden.Error()) {
			t.Fatalf("expected wrapped ErrPostgresForbidden, got %v", err)
		}
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("expected error to mention DATABASE_URL, got %v", err)
	}
	if !strings.Contains(err.Error(), "Postgres DSN") {
		t.Errorf("expected error to classify as Postgres DSN, got %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("expected password to be redacted, got %v", err)
	}
}

func TestCheckEnvPostgresHostBareValueRejected(t *testing.T) {
	env := map[string]string{
		"POSTGRES_HOST": "postgres.hanzo.svc",
	}
	err := CheckEnv(mapLookup(env))
	if err == nil {
		t.Fatal("expected error for bare POSTGRES_HOST")
	}
	if !strings.Contains(err.Error(), "POSTGRES_HOST") {
		t.Errorf("expected error to mention POSTGRES_HOST, got %v", err)
	}
}

func TestCheckEnvMultipleViolationsAllReported(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":        "postgresql://u:p@h:5432/d",
		"NOTIFY_DATABASE_URL": "postgres://u:p@h:5432/d2",
	}
	err := CheckEnv(mapLookup(env))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") || !strings.Contains(err.Error(), "NOTIFY_DATABASE_URL") {
		t.Errorf("expected both vars in error, got %v", err)
	}
}

func TestCheckEnvWhitespaceTrimmed(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL": "   ",
	}
	if err := CheckEnv(mapLookup(env)); err != nil {
		t.Fatalf("expected nil for whitespace-only value, got %v", err)
	}
}

func TestIsPostgres(t *testing.T) {
	cases := map[string]bool{
		"postgres://u@h/d":     true,
		"postgresql://u@h/d":   true,
		"  POSTGRES://U@H/D  ": true,
		"sqlite:///data/x.db":  false,
		"file:///data/x.db":    false,
		"":                     false,
	}
	for v, want := range cases {
		if got := IsPostgres(v); got != want {
			t.Errorf("IsPostgres(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestRedactStripsCredentials(t *testing.T) {
	in := "postgres://hanzo:supersecret@host:5432/db?sslmode=disable"
	out := redact(in)
	if strings.Contains(out, "supersecret") {
		t.Errorf("redact did not strip password: %s", out)
	}
	if !strings.HasPrefix(out, "postgres://***@") {
		t.Errorf("expected postgres://***@ prefix, got %s", out)
	}
	if !strings.HasSuffix(out, "/db?sslmode=disable") {
		t.Errorf("expected trailing dbname to survive, got %s", out)
	}
}

func TestRedactPassthroughForNonDSN(t *testing.T) {
	in := "postgres.hanzo.svc"
	if got := redact(in); got != in {
		t.Errorf("redact mutated non-DSN value: %s → %s", in, got)
	}
}

// mapLookup builds a lookup function from a map literal so tests don't
// have to mutate os.Environ (which is shared global state and creates
// goroutine-unsafe coupling between tests).
func mapLookup(env map[string]string) func(string) string {
	return func(k string) string {
		return env[k]
	}
}
