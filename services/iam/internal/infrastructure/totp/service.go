// Package totp provides TOTP (Time-based One-Time Password) functionality for 2FA.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP spec requires SHA1 per RFC 6238
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
)

// Service provides TOTP operations.
type Service struct {
	issuer string
	digits int
	period int
}

// NewService creates a new TOTP service.
func NewService(cfg *config.TOTPConfig) *Service {
	return &Service{
		issuer: cfg.Issuer,
		digits: cfg.Digits,
		period: cfg.Period,
	}
}

// GenerateSecret generates a new TOTP secret.
func (s *Service) GenerateSecret() (string, error) {
	// Generate 20 random bytes for the secret
	secret := make([]byte, 20)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), nil
}

// GenerateQRURI generates a URI for QR code scanning.
func (s *Service) GenerateQRURI(secret, accountName string) string {
	return fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		s.issuer, accountName, secret, s.issuer, s.digits, s.period,
	)
}

// Validate validates a TOTP code.
func (s *Service) Validate(secret, code string) bool {
	// Allow for clock skew by checking previous and next periods
	now := time.Now().Unix()
	periods := []int64{
		now / int64(s.period),
		now/int64(s.period) - 1,
		now/int64(s.period) + 1,
	}

	for _, period := range periods {
		expectedCode := s.generateCode(secret, period)
		if expectedCode == code {
			return true
		}
	}
	return false
}

func (s *Service) generateCode(secret string, period int64) string {
	// Decode secret
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}

	// Convert period to bytes
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(period)) //nolint:gosec // period is always positive (unix timestamp / period)

	// Calculate HMAC-SHA1
	mac := hmac.New(sha1.New, secretBytes)
	mac.Write(buf)
	hash := mac.Sum(nil)

	// Dynamic truncation
	offset := hash[len(hash)-1] & 0x0f
	code := binary.BigEndian.Uint32(hash[offset:offset+4]) & 0x7fffffff
	code %= pow10(s.digits)

	return fmt.Sprintf("%0*d", s.digits, code)
}

func pow10(n int) uint32 {
	result := uint32(1)
	for i := 0; i < n; i++ {
		result *= 10
	}
	return result
}
