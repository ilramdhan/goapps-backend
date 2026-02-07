// Package session provides domain logic for user session management.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// Session represents an active user session.
type Session struct {
	id               uuid.UUID
	userID           uuid.UUID
	refreshTokenHash string
	deviceInfo       string
	ipAddress        string
	serviceName      string
	expiresAt        time.Time
	createdAt        time.Time
	revokedAt        *time.Time
}

// NewSession creates a new Session entity.
func NewSession(
	userID uuid.UUID,
	refreshToken string,
	deviceInfo, ipAddress, serviceName string,
	expiresAt time.Time,
) *Session {
	return &Session{
		id:               uuid.New(),
		userID:           userID,
		refreshTokenHash: hashToken(refreshToken),
		deviceInfo:       deviceInfo,
		ipAddress:        ipAddress,
		serviceName:      serviceName,
		expiresAt:        expiresAt,
		createdAt:        time.Now(),
	}
}

// ReconstructSession reconstructs a Session from persistence.
func ReconstructSession(
	id, userID uuid.UUID,
	refreshTokenHash, deviceInfo, ipAddress, serviceName string,
	expiresAt, createdAt time.Time,
	revokedAt *time.Time,
) *Session {
	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: refreshTokenHash,
		deviceInfo:       deviceInfo,
		ipAddress:        ipAddress,
		serviceName:      serviceName,
		expiresAt:        expiresAt,
		createdAt:        createdAt,
		revokedAt:        revokedAt,
	}
}

// ID returns the session identifier.
func (s *Session) ID() uuid.UUID { return s.id }

// UserID returns the user identifier.
func (s *Session) UserID() uuid.UUID { return s.userID }

// RefreshTokenHash returns the refresh token hash.
func (s *Session) RefreshTokenHash() string { return s.refreshTokenHash }

// DeviceInfo returns the device information.
func (s *Session) DeviceInfo() string { return s.deviceInfo }

// IPAddress returns the IP address.
func (s *Session) IPAddress() string { return s.ipAddress }

// ServiceName returns the service name.
func (s *Session) ServiceName() string { return s.serviceName }

// ExpiresAt returns the expiration time.
func (s *Session) ExpiresAt() time.Time { return s.expiresAt }

// CreatedAt returns the creation time.
func (s *Session) CreatedAt() time.Time { return s.createdAt }

// RevokedAt returns the revocation time.
func (s *Session) RevokedAt() *time.Time { return s.revokedAt }

// IsActive returns true if the session is not revoked and not expired.
func (s *Session) IsActive() bool {
	return s.revokedAt == nil && time.Now().Before(s.expiresAt)
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.expiresAt)
}

// IsRevoked returns true if the session has been revoked.
func (s *Session) IsRevoked() bool {
	return s.revokedAt != nil
}

// Revoke marks the session as revoked.
func (s *Session) Revoke() {
	now := time.Now()
	s.revokedAt = &now
}

// ValidateToken validates a refresh token against the stored hash.
func (s *Session) ValidateToken(token string) bool {
	return hashToken(token) == s.refreshTokenHash
}

// UpdateTokenID updates the session with a new token and expiry.
func (s *Session) UpdateTokenID(newTokenID string, newExpiresAt time.Time) {
	s.refreshTokenHash = hashToken(newTokenID)
	s.expiresAt = newExpiresAt
}

// hashToken creates a SHA256 hash of a token.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// Info contains minimal session information for display.
type Info struct {
	SessionID   uuid.UUID
	UserID      uuid.UUID
	Username    string
	FullName    string
	DeviceInfo  string
	IPAddress   string
	ServiceName string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}
