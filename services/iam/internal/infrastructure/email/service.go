// Package email provides SMTP email sending for the IAM service.
package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
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
	cfg      *config.EmailConfig
	renderer *Renderer
}

// NewService creates a new email service with the given config and renderer.
func NewService(cfg *config.EmailConfig, renderer *Renderer) *Service {
	return &Service{cfg: cfg, renderer: renderer}
}

// SendOTP sends a password reset OTP to the user's email.
func (s *Service) SendOTP(ctx context.Context, email, otp string, expiryMinutes int) error {
	base := s.renderer.BaseData()
	base.HeaderTitle = "Password Reset"
	base.HeaderSubtitle = "One-time verification code"
	data := OTPData{
		BaseData:       base,
		RecipientEmail: email,
		OTPDigits:      SplitOTP(otp),
		ExpiryMinutes:  expiryMinutes,
		Purpose:        "password reset",
	}
	body, err := s.renderer.Render("otp", data)
	if err != nil {
		return fmt.Errorf("render OTP email: %w", err)
	}
	return s.send(ctx, email, "Your password reset code", body)
}

// SendEmailVerification sends an email verification OTP to the user's email.
func (s *Service) SendEmailVerification(ctx context.Context, email, otp string, expiryMinutes int) error {
	base := s.renderer.BaseData()
	base.HeaderTitle = "Verify Your Email"
	base.HeaderSubtitle = "One-time verification code"
	data := OTPData{
		BaseData:       base,
		RecipientEmail: email,
		OTPDigits:      SplitOTP(otp),
		ExpiryMinutes:  expiryMinutes,
		Purpose:        "email verification",
	}
	body, err := s.renderer.Render("otp", data)
	if err != nil {
		return fmt.Errorf("render email verification email: %w", err)
	}
	return s.send(ctx, email, "Verify your email address", body)
}

// Send2FANotification sends a notification about a 2FA status change.
func (s *Service) Send2FANotification(ctx context.Context, email, action string) error {
	base := s.renderer.BaseData()
	base.HeaderTitle = "Security Update"
	base.HeaderSubtitle = "Account security notification"
	data := SecurityData{
		BaseData:  base,
		Feature:   "Two-Factor Authentication",
		Action:    action,
		SecureURL: s.cfg.AppURL + "/settings/security",
	}
	body, err := s.renderer.Render("security", data)
	if err != nil {
		return fmt.Errorf("render 2FA notification email: %w", err)
	}
	subject := fmt.Sprintf("Two-factor authentication %s", action)
	return s.send(ctx, email, subject, body)
}

// SendWelcomeUser sends a welcome email to a newly created user.
func (s *Service) SendWelcomeUser(ctx context.Context, toEmail, toName string) error {
	base := s.renderer.BaseData()
	base.HeaderTitle = "Welcome to " + s.cfg.AppName
	base.HeaderSubtitle = "Your account is ready"
	data := WelcomeData{
		BaseData:       base,
		RecipientName:  toName,
		RecipientEmail: toEmail,
		LoginURL:       s.cfg.AppURL,
	}
	body, err := s.renderer.Render("welcome", data)
	if err != nil {
		return fmt.Errorf("render welcome email: %w", err)
	}
	return s.send(ctx, toEmail, "Welcome to "+s.cfg.AppName, body)
}

// AppURL returns the configured application base URL used for constructing CTA links.
func (s *Service) AppURL() string {
	return s.cfg.AppURL
}

// SendNotification sends a general platform notification email.
// ctaURL is optional; when non-empty a "View Details" button is shown in the email.
// When SMTP host is unconfigured, this is a no-op.
func (s *Service) SendNotification(ctx context.Context, toEmail, toName, title, body, ctaURL string) error {
	if s.cfg.SMTPHost == "" {
		return nil
	}
	base := s.renderer.BaseData()
	base.HeaderTitle = title
	data := NotificationData{
		BaseData:      base,
		RecipientName: toName,
		Title:         title,
		Paragraphs:    SplitParagraphs(body),
	}
	if ctaURL != "" {
		data.CTA = CTAData{Label: "View Details", URL: ctaURL}
	}
	htmlBody, err := s.renderer.Render("notification", data)
	if err != nil {
		return fmt.Errorf("render notification email: %w", err)
	}
	return s.send(ctx, toEmail, title, htmlBody)
}

// SendNotificationWithAttachments sends a notification email with file attachments.
// Used for export-ready notifications (RM Cost export, bulk exports, etc.).
func (s *Service) SendNotificationWithAttachments(
	ctx context.Context,
	toEmail, toName, title, body string,
	attachments ...Attachment,
) error {
	if s.cfg.SMTPHost == "" {
		return nil
	}
	base := s.renderer.BaseData()
	base.HeaderTitle = title
	data := NotificationData{
		BaseData:      base,
		RecipientName: toName,
		Title:         title,
		Paragraphs:    SplitParagraphs(body),
	}
	htmlBody, err := s.renderer.Render("notification", data)
	if err != nil {
		return fmt.Errorf("render notification-with-attachments email: %w", err)
	}
	return s.send(ctx, toEmail, title, htmlBody, attachments...)
}

// SendNotificationWithTable sends a notification email with an inline data table.
// Used for approval summaries, status digests, and workflow notifications.
func (s *Service) SendNotificationWithTable(
	ctx context.Context,
	toEmail, toName, title, body string,
	table TableData,
	cta CTAData,
) error {
	if s.cfg.SMTPHost == "" {
		return nil
	}
	base := s.renderer.BaseData()
	base.HeaderTitle = title
	data := NotificationData{
		BaseData:      base,
		RecipientName: toName,
		Title:         title,
		Paragraphs:    SplitParagraphs(body),
		Table:         &table,
		CTA:           cta,
	}
	htmlBody, err := s.renderer.Render("notification", data)
	if err != nil {
		return fmt.Errorf("render notification-with-table email: %w", err)
	}
	return s.send(ctx, toEmail, title, htmlBody)
}

func (s *Service) send(ctx context.Context, to, subject, htmlBody string, attachments ...Attachment) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)

	headers := map[string]string{
		"From":    fmt.Sprintf("%s <%s>", s.cfg.FromName, s.cfg.FromAddress),
		"To":      to,
		"Subject": subject,
	}

	msg := buildMessage(headers, htmlBody, attachments)

	var auth smtp.Auth
	if s.cfg.SMTPUser != "" && s.cfg.SMTPPassword != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUser, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	}

	if s.cfg.UseTLS {
		return s.sendTLS(ctx, addr, auth, to, msg)
	}

	if err := smtp.SendMail(addr, auth, s.cfg.FromAddress, []string{to}, []byte(msg)); err != nil {
		log.Error().Err(err).Str("to", to).Str("subject", subject).Msg("Failed to send email")
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Info().Str("to", to).Str("subject", subject).Msg("Email sent successfully")
	return nil
}

func (s *Service) sendTLS(ctx context.Context, addr string, auth smtp.Auth, to, msg string) error {
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

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			if closeErr := conn.Close(); closeErr != nil {
				log.Warn().Err(closeErr).Msg("failed to close SMTP connection after SetDeadline error")
			}
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
