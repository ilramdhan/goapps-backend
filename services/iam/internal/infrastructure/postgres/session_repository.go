// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// SessionRepository implements session.Repository interface.
type SessionRepository struct {
	db *DB
}

// NewSessionRepository creates a new SessionRepository.
func NewSessionRepository(db *DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create creates a new session and revokes all previous sessions for the user (single device policy).
func (r *SessionRepository) Create(ctx context.Context, s *session.Session) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		// Revoke all existing active sessions for this user (single device login policy)
		revokeQuery := `
			UPDATE user_sessions SET revoked_at = $1
			WHERE user_id = $2 AND revoked_at IS NULL AND expires_at > $1
		`
		if _, err := tx.ExecContext(ctx, revokeQuery, time.Now(), s.UserID()); err != nil {
			return fmt.Errorf("failed to revoke existing sessions: %w", err)
		}

		// Insert new session
		insertQuery := `
			INSERT INTO user_sessions (
				session_id, user_id, refresh_token_hash, device_info, ip_address,
				service_name, expires_at, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`
		_, err := tx.ExecContext(ctx, insertQuery,
			s.ID(), s.UserID(), s.RefreshTokenHash(), s.DeviceInfo(), s.IPAddress(),
			s.ServiceName(), s.ExpiresAt(), s.CreatedAt(),
		)
		if err != nil {
			return fmt.Errorf("failed to insert session: %w", err)
		}

		return nil
	})
}

// GetByID retrieves a session by ID.
func (r *SessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	query := `
		SELECT session_id, user_id, refresh_token_hash, device_info, ip_address,
			service_name, expires_at, created_at, revoked_at
		FROM user_sessions
		WHERE session_id = $1
	`

	var s sessionRow
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.UserID, &s.RefreshTokenHash, &s.DeviceInfo, &s.IPAddress,
		&s.ServiceName, &s.ExpiresAt, &s.CreatedAt, &s.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return s.toDomain(), nil
}

// GetByRefreshToken retrieves a session by refresh token hash.
func (r *SessionRepository) GetByRefreshToken(ctx context.Context, tokenHash string) (*session.Session, error) {
	query := `
		SELECT session_id, user_id, refresh_token_hash, device_info, ip_address,
			service_name, expires_at, created_at, revoked_at
		FROM user_sessions
		WHERE refresh_token_hash = $1
	`

	var s sessionRow
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&s.ID, &s.UserID, &s.RefreshTokenHash, &s.DeviceInfo, &s.IPAddress,
		&s.ServiceName, &s.ExpiresAt, &s.CreatedAt, &s.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return s.toDomain(), nil
}

// GetActiveByUserID retrieves the active session for a user.
func (r *SessionRepository) GetActiveByUserID(ctx context.Context, userID uuid.UUID) (*session.Session, error) {
	query := `
		SELECT session_id, user_id, refresh_token_hash, device_info, ip_address,
			service_name, expires_at, created_at, revoked_at
		FROM user_sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var s sessionRow
	err := r.db.QueryRowContext(ctx, query, userID, time.Now()).Scan(
		&s.ID, &s.UserID, &s.RefreshTokenHash, &s.DeviceInfo, &s.IPAddress,
		&s.ServiceName, &s.ExpiresAt, &s.CreatedAt, &s.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	return s.toDomain(), nil
}

// Revoke revokes a session by ID.
func (r *SessionRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE user_sessions SET revoked_at = $2 WHERE session_id = $1`
	result, err := r.db.ExecContext(ctx, query, id, time.Now())
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// GetByTokenID retrieves a session by refresh token hash (JWT token ID stored as hash).
func (r *SessionRepository) GetByTokenID(ctx context.Context, tokenID string) (*session.Session, error) {
	// Token ID is stored as hash in refresh_token_hash
	return r.GetByRefreshToken(ctx, hashToken(tokenID))
}

// Update updates a session.
func (r *SessionRepository) Update(ctx context.Context, s *session.Session) error {
	query := `
		UPDATE user_sessions SET
			refresh_token_hash = $2, expires_at = $3, revoked_at = $4
		WHERE session_id = $1
	`
	result, err := r.db.ExecContext(ctx, query,
		s.ID(), s.RefreshTokenHash(), s.ExpiresAt(), s.RevokedAt(),
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// RevokeByTokenID revokes a session by JWT token ID (refresh token hash).
func (r *SessionRepository) RevokeByTokenID(ctx context.Context, tokenID string) error {
	query := `UPDATE user_sessions SET revoked_at = $2 WHERE refresh_token_hash = $1 AND revoked_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, hashToken(tokenID), time.Now())
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}

	return nil
}

// RevokeAllForUser revokes all sessions for a user.
func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE user_sessions SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, userID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to revoke all sessions: %w", err)
	}
	return nil
}

// hashToken creates SHA256 hash for consistent token storage lookup.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// ListActive lists all active sessions with pagination.
func (r *SessionRepository) ListActive(ctx context.Context, params session.ListParams) ([]*session.Info, int64, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, "s.revoked_at IS NULL")
	conditions = append(conditions, fmt.Sprintf("s.expires_at > $%d", argPos))
	args = append(args, time.Now())
	argPos++

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(u.username ILIKE $%d OR d.full_name ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}

	if params.ServiceName != "" {
		conditions = append(conditions, fmt.Sprintf("s.service_name = $%d", argPos))
		args = append(args, params.ServiceName)
		argPos++
	}

	if params.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("s.user_id = $%d", argPos))
		args = append(args, *params.UserID)
		argPos++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM user_sessions s
		LEFT JOIN mst_user u ON s.user_id = u.user_id
		LEFT JOIN mst_user_detail d ON u.user_id = d.user_id
		WHERE %s
	`, whereClause)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count sessions: %w", err)
	}

	// Build query
	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT s.session_id, s.user_id, u.username, COALESCE(d.full_name, u.username),
			s.device_info, s.ip_address, s.service_name, s.created_at, s.expires_at, s.revoked_at
		FROM user_sessions s
		LEFT JOIN mst_user u ON s.user_id = u.user_id
		LEFT JOIN mst_user_detail d ON u.user_id = d.user_id
		WHERE %s
		ORDER BY s.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argPos, argPos+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in session list")
		}
	}()

	var sessions []*session.Info
	for rows.Next() {
		var s session.Info
		if err := rows.Scan(
			&s.SessionID, &s.UserID, &s.Username, &s.FullName,
			&s.DeviceInfo, &s.IPAddress, &s.ServiceName, &s.CreatedAt, &s.ExpiresAt, &s.RevokedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, &s)
	}

	return sessions, total, nil
}

// CleanupExpired removes expired sessions.
func (r *SessionRepository) CleanupExpired(ctx context.Context) (int, error) {
	query := `DELETE FROM user_sessions WHERE expires_at < $1`
	result, err := r.db.ExecContext(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired sessions: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rows), nil
}

// Helper struct for scanning
type sessionRow struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	DeviceInfo       sql.NullString
	IPAddress        sql.NullString
	ServiceName      string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
}

func (r *sessionRow) toDomain() *session.Session {
	return session.ReconstructSession(
		r.ID, r.UserID, r.RefreshTokenHash, r.DeviceInfo.String, r.IPAddress.String,
		r.ServiceName, r.ExpiresAt, r.CreatedAt, r.RevokedAt,
	)
}
