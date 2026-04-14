// Package email provides SMTP email sending for the IAM service.
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
)

// SMTP client timeouts. Kept tight so failures surface fast and never block the request path.
const (
	smtpDialTimeout    = 10 * time.Second
	smtpOverallTimeout = 30 * time.Second
)

// Service implements the auth.EmailService interface via SMTP.
type Service struct {
	cfg *config.EmailConfig
}

// NewService creates a new email service.
func NewService(cfg *config.EmailConfig) *Service {
	return &Service{cfg: cfg}
}

// SendOTP sends a password reset OTP to the user's email.
func (s *Service) SendOTP(ctx context.Context, email, otp string, expiryMinutes int) error {
	subject := "GoApps - Password Reset OTP"
	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
  <h2 style="color: #333;">Password Reset</h2>
  <p>Your OTP code for password reset is:</p>
  <div style="background: #f4f4f4; padding: 20px; text-align: center; margin: 20px 0; border-radius: 8px;">
    <span style="font-size: 32px; font-weight: bold; letter-spacing: 8px; color: #2563eb;">%s</span>
  </div>
  <p>This code expires in <strong>%d minutes</strong>.</p>
  <p style="color: #666; font-size: 12px;">If you did not request this, please ignore this email.</p>
</body>
</html>`, otp, expiryMinutes)

	return s.send(ctx, email, subject, body)
}

// Send2FANotification sends a notification about 2FA status change.
func (s *Service) Send2FANotification(ctx context.Context, email, action string) error {
	subject := "GoApps - Two-Factor Authentication Update"
	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
  <h2 style="color: #333;">Two-Factor Authentication</h2>
  <p>Two-factor authentication has been <strong>%s</strong> on your account.</p>
  <p style="color: #666; font-size: 12px;">If you did not make this change, please contact support immediately.</p>
</body>
</html>`, action)

	return s.send(ctx, email, subject, body)
}

func (s *Service) send(ctx context.Context, to, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	headers := map[string]string{
		"From":         fmt.Sprintf("%s <%s>", s.cfg.FromName, s.cfg.FromAddress),
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	var msg strings.Builder
	for k, v := range headers {
		_, _ = fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	// Determine auth (nil if no user/password — works for Mailpit)
	var auth smtp.Auth
	if s.cfg.SMTPUser != "" && s.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	}

	if s.cfg.UseTLS {
		return s.sendTLS(ctx, addr, auth, to, msg.String())
	}

	err := smtp.SendMail(addr, auth, s.cfg.FromAddress, []string{to}, []byte(msg.String()))
	if err != nil {
		log.Error().Err(err).Str("to", to).Str("subject", subject).Msg("Failed to send email")
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Info().Str("to", to).Str("subject", subject).Msg("Email sent successfully")
	return nil
}

func (s *Service) sendTLS(ctx context.Context, addr string, auth smtp.Auth, to, msg string) error {
	// Enforce a bounded overall deadline so no SMTP step can hang indefinitely.
	ctx, cancel := context.WithTimeout(ctx, smtpOverallTimeout)
	defer cancel()

	tlsConfig := &tls.Config{
		ServerName: s.cfg.SMTPHost,
		MinVersion: tls.VersionTLS12,
	}
	if s.cfg.SkipVerify {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec // Configurable for dev/self-hosted environments.
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: smtpDialTimeout},
		Config:    tlsConfig,
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	// After dial, TCP-level deadlines prevent a stalled server from hanging reads/writes.
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			_ = conn.Close()
			return fmt.Errorf("failed to set SMTP connection deadline: %w", err)
		}
	}

	client, err := smtp.NewClient(conn, s.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close SMTP client")
		}
	}()

	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err = client.Mail(s.cfg.FromAddress); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err = w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("SMTP close failed: %w", err)
	}

	log.Info().Str("to", to).Msg("Email sent successfully via TLS")
	return client.Quit()
}
