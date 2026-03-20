// Package session_test provides unit tests for the Session domain entity.
package session_test

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
)

// helper to compute expected token hash.
func testHashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// =============================================================================
// NewSession
// =============================================================================

func TestNewSession(t *testing.T) {
	t.Run("valid creation", func(t *testing.T) {
		userID := uuid.New()
		refreshToken := "my-refresh-token"
		deviceInfo := "Chrome on Linux"
		ipAddress := "192.168.1.1"
		serviceName := "iam"
		expiresAt := time.Now().Add(24 * time.Hour)

		s := session.NewSession(userID, refreshToken, deviceInfo, ipAddress, serviceName, expiresAt)

		require.NotNil(t, s)
		assert.NotEqual(t, uuid.Nil, s.ID())
		assert.Equal(t, userID, s.UserID())
		assert.Equal(t, testHashToken(refreshToken), s.RefreshTokenHash())
		assert.Equal(t, deviceInfo, s.DeviceInfo())
		assert.Equal(t, ipAddress, s.IPAddress())
		assert.Equal(t, serviceName, s.ServiceName())
		assert.Equal(t, expiresAt, s.ExpiresAt())
		assert.WithinDuration(t, time.Now(), s.CreatedAt(), 2*time.Second)
		assert.WithinDuration(t, time.Now(), s.LastActivityAt(), 2*time.Second)
		assert.Nil(t, s.RevokedAt())
	})

	t.Run("empty device info", func(t *testing.T) {
		userID := uuid.New()
		expiresAt := time.Now().Add(time.Hour)

		s := session.NewSession(userID, "token", "", "127.0.0.1", "iam", expiresAt)

		require.NotNil(t, s)
		assert.Equal(t, "", s.DeviceInfo())
	})

	t.Run("token is hashed not stored raw", func(t *testing.T) {
		rawToken := "super-secret-token"
		s := session.NewSession(uuid.New(), rawToken, "device", "10.0.0.1", "iam", time.Now().Add(time.Hour))

		assert.NotEqual(t, rawToken, s.RefreshTokenHash())
		assert.Equal(t, testHashToken(rawToken), s.RefreshTokenHash())
	})
}

// =============================================================================
// ReconstructSession
// =============================================================================

func TestReconstructSession(t *testing.T) {
	t.Run("reconstruct without revocation", func(t *testing.T) {
		id := uuid.New()
		userID := uuid.New()
		tokenHash := testHashToken("some-token")
		deviceInfo := "Firefox on macOS"
		ipAddress := "10.0.0.5"
		serviceName := "finance"
		expiresAt := time.Now().Add(12 * time.Hour)
		createdAt := time.Now().Add(-1 * time.Hour)

		s := session.ReconstructSession(id, userID, tokenHash, deviceInfo, ipAddress, serviceName, expiresAt, createdAt, nil, createdAt)

		require.NotNil(t, s)
		assert.Equal(t, id, s.ID())
		assert.Equal(t, userID, s.UserID())
		assert.Equal(t, tokenHash, s.RefreshTokenHash())
		assert.Equal(t, deviceInfo, s.DeviceInfo())
		assert.Equal(t, ipAddress, s.IPAddress())
		assert.Equal(t, serviceName, s.ServiceName())
		assert.Equal(t, expiresAt, s.ExpiresAt())
		assert.Equal(t, createdAt, s.CreatedAt())
		assert.Nil(t, s.RevokedAt())
	})

	t.Run("reconstruct with revocation", func(t *testing.T) {
		revokedAt := time.Now().Add(-30 * time.Minute)

		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now().Add(-2*time.Hour), &revokedAt, time.Now().Add(-2*time.Hour),
		)

		require.NotNil(t, s)
		require.NotNil(t, s.RevokedAt())
		assert.Equal(t, revokedAt, *s.RevokedAt())
	})
}

// =============================================================================
// Getters
// =============================================================================

func TestSession_Getters(t *testing.T) {
	id := uuid.New()
	userID := uuid.New()
	tokenHash := testHashToken("tok")
	device := "Safari on iOS"
	ip := "172.16.0.1"
	service := "iam"
	expires := time.Now().Add(6 * time.Hour)
	created := time.Now().Add(-10 * time.Minute)

	s := session.ReconstructSession(id, userID, tokenHash, device, ip, service, expires, created, nil, created)

	assert.Equal(t, id, s.ID())
	assert.Equal(t, userID, s.UserID())
	assert.Equal(t, tokenHash, s.RefreshTokenHash())
	assert.Equal(t, device, s.DeviceInfo())
	assert.Equal(t, ip, s.IPAddress())
	assert.Equal(t, service, s.ServiceName())
	assert.Equal(t, expires, s.ExpiresAt())
	assert.Equal(t, created, s.CreatedAt())
	assert.Nil(t, s.RevokedAt())
}

// =============================================================================
// IsActive
// =============================================================================

func TestSession_IsActive(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		revokedAt *time.Time
		want      bool
	}{
		{
			name:      "active - not expired and not revoked",
			expiresAt: time.Now().Add(time.Hour),
			revokedAt: nil,
			want:      true,
		},
		{
			name:      "inactive - expired",
			expiresAt: time.Now().Add(-time.Hour),
			revokedAt: nil,
			want:      false,
		},
		{
			name:      "inactive - revoked",
			expiresAt: time.Now().Add(time.Hour),
			revokedAt: timePtr(time.Now().Add(-10 * time.Minute)),
			want:      false,
		},
		{
			name:      "inactive - both expired and revoked",
			expiresAt: time.Now().Add(-time.Hour),
			revokedAt: timePtr(time.Now().Add(-2 * time.Hour)),
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := session.ReconstructSession(
				uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
				tc.expiresAt, time.Now().Add(-time.Hour), tc.revokedAt, time.Now().Add(-time.Hour),
			)

			assert.Equal(t, tc.want, s.IsActive())
		})
	}
}

// =============================================================================
// IsExpired
// =============================================================================

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired - future expiry",
			expiresAt: time.Now().Add(time.Hour),
			want:      false,
		},
		{
			name:      "expired - past expiry",
			expiresAt: time.Now().Add(-time.Hour),
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := session.ReconstructSession(
				uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
				tc.expiresAt, time.Now().Add(-time.Hour), nil, time.Now().Add(-time.Hour),
			)

			assert.Equal(t, tc.want, s.IsExpired())
		})
	}
}

// =============================================================================
// IsRevoked
// =============================================================================

func TestSession_IsRevoked(t *testing.T) {
	t.Run("not revoked", func(t *testing.T) {
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now(), nil, time.Now(),
		)

		assert.False(t, s.IsRevoked())
	})

	t.Run("revoked", func(t *testing.T) {
		revoked := time.Now()
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now(), &revoked, time.Now(),
		)

		assert.True(t, s.IsRevoked())
	})
}

// =============================================================================
// Revoke
// =============================================================================

func TestSession_Revoke(t *testing.T) {
	t.Run("revoke active session", func(t *testing.T) {
		s := session.NewSession(uuid.New(), "token", "device", "1.2.3.4", "iam", time.Now().Add(time.Hour))

		assert.Nil(t, s.RevokedAt())
		assert.False(t, s.IsRevoked())

		s.Revoke()

		assert.NotNil(t, s.RevokedAt())
		assert.True(t, s.IsRevoked())
		assert.False(t, s.IsActive())
		assert.WithinDuration(t, time.Now(), *s.RevokedAt(), 2*time.Second)
	})

	t.Run("revoke already revoked session overwrites timestamp", func(t *testing.T) {
		s := session.NewSession(uuid.New(), "token", "device", "1.2.3.4", "iam", time.Now().Add(time.Hour))

		s.Revoke()
		firstRevokedAt := *s.RevokedAt()

		// Small delay to differentiate timestamps.
		time.Sleep(5 * time.Millisecond)

		s.Revoke()
		secondRevokedAt := *s.RevokedAt()

		assert.True(t, s.IsRevoked())
		assert.True(t, secondRevokedAt.After(firstRevokedAt) || secondRevokedAt.Equal(firstRevokedAt))
	})

	t.Run("revoke expired session", func(t *testing.T) {
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour), nil, time.Now().Add(-2*time.Hour),
		)

		assert.True(t, s.IsExpired())
		assert.False(t, s.IsRevoked())

		s.Revoke()

		assert.True(t, s.IsRevoked())
		assert.NotNil(t, s.RevokedAt())
	})
}

// =============================================================================
// ValidateToken
// =============================================================================

func TestSession_ValidateToken(t *testing.T) {
	tests := []struct {
		name       string
		storeToken string
		checkToken string
		want       bool
	}{
		{
			name:       "matching token",
			storeToken: "my-refresh-token",
			checkToken: "my-refresh-token",
			want:       true,
		},
		{
			name:       "non-matching token",
			storeToken: "my-refresh-token",
			checkToken: "wrong-token",
			want:       false,
		},
		{
			name:       "empty token against non-empty",
			storeToken: "my-refresh-token",
			checkToken: "",
			want:       false,
		},
		{
			name:       "empty token against empty",
			storeToken: "",
			checkToken: "",
			want:       true,
		},
		{
			name:       "case sensitive",
			storeToken: "Token",
			checkToken: "token",
			want:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := session.NewSession(uuid.New(), tc.storeToken, "device", "1.2.3.4", "iam", time.Now().Add(time.Hour))

			assert.Equal(t, tc.want, s.ValidateToken(tc.checkToken))
		})
	}
}

// =============================================================================
// UpdateTokenID
// =============================================================================

func TestSession_UpdateTokenID(t *testing.T) {
	t.Run("updates hash and expiry", func(t *testing.T) {
		originalToken := "original-token"
		s := session.NewSession(uuid.New(), originalToken, "device", "1.2.3.4", "iam", time.Now().Add(time.Hour))

		originalHash := s.RefreshTokenHash()
		originalExpiry := s.ExpiresAt()

		newToken := "new-token"
		newExpiry := time.Now().Add(48 * time.Hour)

		s.UpdateTokenID(newToken, newExpiry)

		assert.NotEqual(t, originalHash, s.RefreshTokenHash())
		assert.Equal(t, testHashToken(newToken), s.RefreshTokenHash())
		assert.NotEqual(t, originalExpiry, s.ExpiresAt())
		assert.Equal(t, newExpiry, s.ExpiresAt())
	})

	t.Run("old token no longer validates", func(t *testing.T) {
		s := session.NewSession(uuid.New(), "old-token", "device", "1.2.3.4", "iam", time.Now().Add(time.Hour))

		assert.True(t, s.ValidateToken("old-token"))

		s.UpdateTokenID("new-token", time.Now().Add(2*time.Hour))

		assert.False(t, s.ValidateToken("old-token"))
		assert.True(t, s.ValidateToken("new-token"))
	})
}

// =============================================================================
// IsIdle
// =============================================================================

func TestSession_IsIdle(t *testing.T) {
	t.Run("not idle - recent activity", func(t *testing.T) {
		s := session.NewSession(uuid.New(), "token", "device", "1.2.3.4", "iam", time.Now().Add(time.Hour))
		assert.False(t, s.IsIdle(2*time.Hour))
	})

	t.Run("idle - activity older than timeout", func(t *testing.T) {
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now().Add(-3*time.Hour), nil, time.Now().Add(-3*time.Hour),
		)
		assert.True(t, s.IsIdle(2*time.Hour))
	})

	t.Run("zero timeout means never idle", func(t *testing.T) {
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now().Add(-24*time.Hour), nil, time.Now().Add(-24*time.Hour),
		)
		assert.False(t, s.IsIdle(0))
	})

	t.Run("negative timeout means never idle", func(t *testing.T) {
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now().Add(-24*time.Hour), nil, time.Now().Add(-24*time.Hour),
		)
		assert.False(t, s.IsIdle(-1*time.Hour))
	})
}

// =============================================================================
// TouchActivity
// =============================================================================

func TestSession_TouchActivity(t *testing.T) {
	t.Run("updates last activity to now", func(t *testing.T) {
		s := session.ReconstructSession(
			uuid.New(), uuid.New(), "hash", "device", "1.2.3.4", "iam",
			time.Now().Add(time.Hour), time.Now().Add(-3*time.Hour), nil, time.Now().Add(-3*time.Hour),
		)

		assert.True(t, s.IsIdle(2*time.Hour))

		s.TouchActivity()

		assert.WithinDuration(t, time.Now(), s.LastActivityAt(), 2*time.Second)
		assert.False(t, s.IsIdle(2*time.Hour))
	})
}

// =============================================================================
// Info struct
// =============================================================================

func TestInfo(t *testing.T) {
	t.Run("info fields are accessible", func(t *testing.T) {
		sessionID := uuid.New()
		userID := uuid.New()
		now := time.Now()
		expires := now.Add(time.Hour)
		revoked := now.Add(-10 * time.Minute)

		info := session.Info{
			SessionID:   sessionID,
			UserID:      userID,
			Username:    "john.doe",
			FullName:    "John Doe",
			DeviceInfo:  "Chrome on Windows",
			IPAddress:   "192.168.0.10",
			ServiceName: "iam",
			CreatedAt:   now,
			ExpiresAt:   expires,
			RevokedAt:   &revoked,
		}

		assert.Equal(t, sessionID, info.SessionID)
		assert.Equal(t, userID, info.UserID)
		assert.Equal(t, "john.doe", info.Username)
		assert.Equal(t, "John Doe", info.FullName)
		assert.Equal(t, "Chrome on Windows", info.DeviceInfo)
		assert.Equal(t, "192.168.0.10", info.IPAddress)
		assert.Equal(t, "iam", info.ServiceName)
		assert.Equal(t, now, info.CreatedAt)
		assert.Equal(t, expires, info.ExpiresAt)
		require.NotNil(t, info.RevokedAt)
		assert.Equal(t, revoked, *info.RevokedAt)
	})

	t.Run("info with nil revoked at", func(t *testing.T) {
		info := session.Info{
			SessionID: uuid.New(),
			UserID:    uuid.New(),
			RevokedAt: nil,
		}

		assert.Nil(t, info.RevokedAt)
	})
}

// =============================================================================
// Helpers
// =============================================================================

func timePtr(t time.Time) *time.Time {
	return &t
}
