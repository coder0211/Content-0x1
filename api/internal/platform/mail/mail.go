// Package mail implements ports.Mailer over plain SMTP (Mailpit locally).
package mail

import (
	"context"
	"fmt"
	"mime"
	"net/smtp"
	"strings"

	"github.com/be/acs/api/internal/platform/config"
	"github.com/be/acs/api/internal/ports"
)

// SMTP sends mail without authentication (internal relay / Mailpit).
type SMTP struct{ cfg config.SMTPConfig }

// New returns an SMTP mailer.
func New(cfg config.SMTPConfig) *SMTP { return &SMTP{cfg: cfg} }

var _ ports.Mailer = (*SMTP)(nil)

// Send delivers a UTF-8 plain-text message.
func (s *SMTP) Send(_ context.Context, to []string, subject, body string) error {
	for _, addr := range append([]string{s.cfg.From}, to...) {
		if strings.ContainsAny(addr, "\r\n") {
			return fmt.Errorf("mail: invalid address")
		}
	}
	msg := "From: " + s.cfg.From + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body
	from := s.cfg.From
	if i := strings.LastIndex(from, "<"); i >= 0 {
		from = strings.TrimSuffix(from[i+1:], ">")
	}
	return smtp.SendMail(fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port), nil, from, to, []byte(msg))
}
