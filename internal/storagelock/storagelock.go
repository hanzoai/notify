// Package storagelock refuses to boot notifyd against Postgres.
//
// notifyd is SQLite-only: the storage substrate is github.com/hanzoai/base,
// which already opens a SQLite database under --dir/data.db. Per the
// Hanzo PG → SQLite migration plan
// (~/work/hanzo/CLAUDE_PG_TO_SQLITE_MIGRATION.md, service #2 — notify),
// the deliverable for notify is "lock it down with config-level
// enforcement (env var rejects DATABASE_URL pointing at PG)" because
// the service is already PG-free at the code level and the only way
// PG can sneak back in is via an env-var override at deploy time.
//
// CheckEnv inspects the process environment for forbidden Postgres
// indicators and returns an error if any are present. Wire it into the
// notifyd main before app.Start() so the misconfiguration crashes the
// pod loudly rather than producing a working-looking instance that
// silently ignores the override.
package storagelock

import (
	"errors"
	"fmt"
	"strings"
)

// forbiddenEnvs is the closed set of env vars that historically
// pointed Hanzo services at Postgres. If any of these are non-empty
// when notifyd boots, it must crash — there is no legitimate reason
// to set them on a SQLite-only binary.
var forbiddenEnvs = []string{
	"DATABASE_URL",
	"POSTGRES_URL",
	"POSTGRES_DSN",
	"POSTGRES_HOST",
	"NOTIFY_DATABASE_URL",
	"NOTIFY_POSTGRES_URL",
}

// pgSchemes is the prefix set that marks a DSN value as a Postgres URL.
// Used to distinguish "DATABASE_URL set but pointing at sqlite://" (which
// would itself be a code smell but isn't a Postgres lockdown violation)
// from the real failure mode: a leaked postgres:// URL.
var pgSchemes = []string{
	"postgres://",
	"postgresql://",
	"postgres+",
}

// ErrPostgresForbidden is returned when an env var pins notifyd at
// Postgres. It is informational; the caller is expected to log the
// underlying violation list (via Violations) and exit non-zero.
var ErrPostgresForbidden = errors.New("notifyd: Postgres is not a supported storage backend")

// Violations enumerates every offending env var found in the supplied
// lookup function. The lookup is parameterised so tests can drive it
// without mutating os.Environ.
//
// A violation is one of:
//
//	{Var: "DATABASE_URL", Value: "postgres://..."}
//	{Var: "POSTGRES_HOST", Value: "postgres.hanzo.svc"}
//
// Any non-empty value for a host/dsn forbidden env counts; values that
// merely indicate sqlite/file paths are also rejected because the only
// supported storage knob is the hanzoai/base --dir flag.
func Violations(lookup func(string) string) []Violation {
	if lookup == nil {
		return nil
	}
	var out []Violation
	for _, k := range forbiddenEnvs {
		v := strings.TrimSpace(lookup(k))
		if v == "" {
			continue
		}
		out = append(out, Violation{Var: k, Value: v})
	}
	return out
}

// IsPostgres reports whether the value parses as a Postgres DSN. Used
// by the formatted error message to make the lockdown report explicit
// about why a given value tripped the check.
func IsPostgres(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	for _, s := range pgSchemes {
		if strings.HasPrefix(v, s) {
			return true
		}
	}
	return false
}

// Violation captures one offending (var, value) pair.
type Violation struct {
	Var   string
	Value string
}

// CheckEnv panics-callable variant: returns nil when the environment is
// clean, or a wrapped ErrPostgresForbidden listing every violation.
// notifyd's main wires this and log.Fatalf's on any non-nil return.
func CheckEnv(lookup func(string) string) error {
	vs := Violations(lookup)
	if len(vs) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%v:", ErrPostgresForbidden)
	for _, v := range vs {
		kind := "set"
		if IsPostgres(v.Value) {
			kind = "Postgres DSN"
		}
		fmt.Fprintf(&b, " %s=%s (%s)", v.Var, redact(v.Value), kind)
	}
	return errors.New(b.String())
}

// redact masks the credentials portion of a DSN so the lockdown log
// doesn't leak passwords. Non-DSN values pass through untouched
// because the var-name alone is the violation signal there.
func redact(v string) string {
	if !IsPostgres(v) {
		return v
	}
	// postgres://user:pass@host:port/db?... → postgres://***@host:port/db?...
	atIdx := strings.LastIndex(v, "@")
	if atIdx < 0 {
		return v
	}
	schemeEnd := strings.Index(v, "://")
	if schemeEnd < 0 || schemeEnd+3 >= atIdx {
		return v
	}
	return v[:schemeEnd+3] + "***" + v[atIdx:]
}
