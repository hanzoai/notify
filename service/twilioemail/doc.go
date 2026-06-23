/*
Package twilioemail provides email notification integration for Twilio's
native Email API (POST https://comms.twilio.com/v1/Emails).

It is the email counterpart to service/twilio (SMS): both authenticate
with the SAME Twilio account credentials (account SID + auth token) via
HTTP Basic. Use this instead of service/sendgrid when a brand sends email
through its Twilio account rather than a separate SendGrid key.

Usage:

	package main

	import (
		"context"
		"log"

		"github.com/hanzoai/notify"
		"github.com/hanzoai/notify/service/twilioemail"
	)

	func main() {
		emailSvc := twilioemail.New("account_sid", "auth_token", "no-reply@send.hanzo.ai", "Hanzo")
		emailSvc.AddReceivers("recipient@example.com")

		notifier := notify.New()
		notifier.UseServices(emailSvc)

		if err := notifier.Send(context.Background(), "subject", "<b>message</b>"); err != nil {
			log.Fatalf("notifier.Send() failed: %s", err.Error())
		}

		log.Println("notification sent")
	}
*/
package twilioemail
