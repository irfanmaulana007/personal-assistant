// Package mailer sends transactional email over SMTP. It is intentionally small
// — the only transactional message the app sends today is a password-reset
// email — but the transport is generic and works with any SMTP provider
// (Gmail app-passwords, SendGrid, Mailgun, …).
package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// Config holds SMTP transport settings.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the envelope + header sender address. Falls back to Username when
	// empty.
	From string
	// FromName is the optional display name shown alongside From.
	FromName string
}

// Mailer sends email over SMTP. A Mailer with no host configured is disabled:
// Enabled reports false and Send returns an error.
type Mailer struct {
	cfg Config
}

// New builds a Mailer from the given config.
func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// Enabled reports whether an SMTP host is configured.
func (m *Mailer) Enabled() bool {
	return m != nil && strings.TrimSpace(m.cfg.Host) != ""
}

// Send delivers a plain-text email to a single recipient.
func (m *Mailer) Send(to, subject, body string) error {
	if !m.Enabled() {
		return fmt.Errorf("smtp is not configured")
	}

	from := strings.TrimSpace(m.cfg.From)
	if from == "" {
		from = m.cfg.Username
	}
	fromHeader := from
	if m.cfg.FromName != "" {
		fromHeader = fmt.Sprintf("%s <%s>", m.cfg.FromName, from)
	}

	msg := buildMessage(fromHeader, to, subject, body)
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.cfg.Port))

	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}

	// Port 465 speaks TLS from the first byte (implicit TLS); every other port
	// (typically 587) upgrades in-band via STARTTLS, which smtp.SendMail handles.
	if m.cfg.Port == 465 {
		return m.sendImplicitTLS(addr, auth, from, to, msg)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, msg)
}

func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// sendImplicitTLS delivers a message over a connection that is TLS-wrapped from
// the start (SMTPS / port 465), which smtp.SendMail's STARTTLS path cannot do.
func (m *Mailer) sendImplicitTLS(addr string, auth smtp.Auth, from, to string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: m.cfg.Host})
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, m.cfg.Host)
	if err != nil {
		return err
	}
	defer c.Close()

	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}
