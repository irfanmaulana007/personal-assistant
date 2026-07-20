package mailer

import (
	"strings"
	"testing"
)

func TestEnabled(t *testing.T) {
	if (&Mailer{}).Enabled() {
		t.Fatal("mailer with no host should be disabled")
	}
	if !New(Config{Host: "smtp.example.com"}).Enabled() {
		t.Fatal("mailer with a host should be enabled")
	}
	var nilMailer *Mailer
	if nilMailer.Enabled() {
		t.Fatal("nil mailer should be disabled")
	}
}

func TestSendDisabledReturnsError(t *testing.T) {
	if err := (&Mailer{}).Send("a@b.com", "hi", "body"); err == nil {
		t.Fatal("expected error when sending from a disabled mailer")
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("Assistant <no-reply@x.com>", "user@y.com", "Subject line", "Hello body"))

	for _, want := range []string{
		"From: Assistant <no-reply@x.com>\r\n",
		"To: user@y.com\r\n",
		"Subject: Subject line\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing header %q\n---\n%s", want, msg)
		}
	}
	// Headers must be separated from the body by a blank line.
	head, body, ok := strings.Cut(msg, "\r\n\r\n")
	if !ok || body != "Hello body" {
		t.Fatalf("headers/body not separated correctly: head=%q body=%q", head, body)
	}
}
