// Package session provides domain logic for user session management.
package session

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for session persistence operations.
type Repository interface {
	// Create creates a new session and revokes all previous sessions for the user (single device policy).
	Create(ctx context.Context, session *Session) error

	// GetByID retrieves a session by ID.
	GetByID(ctx context.Context, id uuid.UUID) (*Session, error)

	// GetByRefreshToken retrieves a session by refresh token hash.
	GetByRefreshToken(ctx context.Context, tokenHash string) (*Session, error)

	// GetByTokenID retrieves a session by JWT token ID (JTI).
	GetByTokenID(ctx context.Context, tokenID string) (*Session, error)

	// GetActiveByUserID retrieves the active session for a user (single device policy).
	GetActiveByUserID(ctx context.Context, userID uuid.UUID) (*Session, error)

	// Update updates a session.
	Update(ctx context.Context, session *Session) error

	// Revoke revokes a session by ID.
	Revoke(ctx context.Context, id uuid.UUID) error

	// RevokeByTokenID revokes a session by JWT token ID (JTI).
	RevokeByTokenID(ctx context.Context, tokenID string) error

	// RevokeAllForUser revokes all sessions for a user.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error

	// ListActive lists all active sessions (admin only).
	ListActive(ctx context.Context, params ListParams) ([]*Info, int64, error)

	// CleanupExpired removes expired sessions.
	CleanupExpired(ctx context.Context) (int, error)
}

// ListParams contains parameters for listing sessions.
type ListParams struct {
	Page        int
	PageSize    int
	Search      string
	ServiceName string
	UserID      *uuid.UUID
	SortBy      string
	SortOrder   string
}

// CacheRepository defines the interface for session caching (Redis).
type CacheRepository interface {
	// StoreSession stores session data in cache for quick validation.
	StoreSession(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID, expiresIn int64) error

	// GetSession retrieves session data from cache.
	GetSession(ctx context.Context, sessionID uuid.UUID) (uuid.UUID, error)

	// DeleteSession removes a session from cache.
	DeleteSession(ctx context.Context, sessionID uuid.UUID) error

	// BlacklistToken adds a token to the blacklist (for logout before expiry).
	BlacklistToken(ctx context.Context, tokenID string, expiresIn int64) error

	// IsBlacklisted checks if a token is blacklisted.
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
}
