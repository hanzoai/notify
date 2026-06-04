# Hanzo Notify — Release Notes

## 2026-06-03 — Phase 1 notification preferences + multi-provider delivery

**PR #6: Per-channel multi-provider retry chain** (6666f29)

Each channel now drives a primary→fallback provider chain rather than a single hard-coded sender. SMS dispatch tries Plivo first and falls back to Twilio; email tries SES API mode first and falls back to SES SMTP. Failures within the chain are retried with backoff and surfaced as `provider_chain_exhausted` only when every leg fails. Consumers (Hanzo IAM, Liquid BD, Liquid ATS) get higher delivery odds without any client change — the chain is fully internal to Notify.

**PR #7: Phase 1 preferences — channel router, unsubscribe, STOP webhook** (3e5dcda)

Introduces three SQLite tables (`preferences`, `subscriptions`, `unsubscribe_tokens`) and a 55-cell channel-routing matrix (11 event classes × 5 channels: email, sms, web, voice, postal). New CRUD surface at `/v1/notify/preferences/*` lets every consumer read and update per-user defaults. HMAC-signed unsubscribe tokens (`/v1/notify/unsubscribe?t=...`) provide one-click opt-out without authentication, and the inbound SMS webhook (`/v1/notify/sms-inbound`) honors STOP / UNSTOP / HELP keywords per carrier compliance. Marketing and transactional classes are routed independently — STOP disables marketing only by default.

**PR #8: SES SendRawEmail + RFC 8058 List-Unsubscribe injection on marketing** (145c99f)

Email dispatch on the SES API path now uses `SendRawEmail` with full MIME so we can inject the RFC 8058 `List-Unsubscribe` and `List-Unsubscribe-Post: List-Unsubscribe=One-Click` headers. The injector runs only when the event class resolves to `marketing` — transactional classes (OTP, security alerts) keep clean headers. Inbox providers (Gmail, Outlook) render the one-click button automatically and honor the HMAC token from PR #7. No flag — the behavior is class-gated.
