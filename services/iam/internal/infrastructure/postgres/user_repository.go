// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/user"
)

// UserRepository implements user.Repository interface.
type UserRepository struct {
	db *DB
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user with detail.
func (r *UserRepository) Create(ctx context.Context, u *user.User, detail *user.Detail) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		// Insert user
		query := `
			INSERT INTO mst_user (
				user_id, username, email, password_hash, is_active, is_locked,
				failed_login_attempts, locked_until, two_factor_enabled, two_factor_secret,
				last_login_at, last_login_ip, password_changed_at,
				created_at, created_by
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		`
		_, err := tx.ExecContext(ctx, query,
			u.ID(), u.Username(), u.Email(), u.PasswordHash(), u.IsActive(), u.IsLocked(),
			u.FailedLoginAttempts(), u.LockedUntil(), u.TwoFactorEnabled(), u.TwoFactorSecret(),
			u.LastLoginAt(), u.LastLoginIP(), u.PasswordChangedAt(),
			u.Audit().CreatedAt, u.Audit().CreatedBy,
		)
		if err != nil {
			return fmt.Errorf("failed to insert user: %w", err)
		}

		// Insert user detail if provided
		if detail != nil {
			extraDataJSON, err := json.Marshal(detail.ExtraData())
			if err != nil {
				return fmt.Errorf("failed to marshal extra data: %w", err)
			}
			query = `
				INSERT INTO mst_user_detail (
					detail_id, user_id, section_id, employee_code, full_name, first_name, last_name,
					phone, profile_picture, position, date_of_birth, address, extra_data,
					created_at, created_by
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			`
			_, err = tx.ExecContext(ctx, query,
				detail.ID(), detail.UserID(), detail.SectionID(), detail.EmployeeCode(),
				detail.FullName(), detail.FirstName(), detail.LastName(),
				detail.Phone(), detail.ProfilePicture(), detail.Position(),
				detail.DateOfBirth(), detail.Address(), extraDataJSON,
				detail.Audit().CreatedAt, detail.Audit().CreatedBy,
			)
			if err != nil {
				return fmt.Errorf("failed to insert user detail: %w", err)
			}
		}

		return nil
	})
}

// GetByID retrieves a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	query := `
		SELECT user_id, username, email, password_hash, is_active, is_locked,
			failed_login_attempts, locked_until, two_factor_enabled, two_factor_secret,
			last_login_at, last_login_ip, password_changed_at,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_user
		WHERE user_id = $1 AND deleted_at IS NULL
	`

	var u userRow
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsActive, &u.IsLocked,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.TwoFactorEnabled, &u.TwoFactorSecret,
		&u.LastLoginAt, &u.LastLoginIP, &u.PasswordChangedAt,
		&u.CreatedAt, &u.CreatedBy, &u.UpdatedAt, &u.UpdatedBy, &u.DeletedAt, &u.DeletedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by id: %w", err)
	}

	return u.toDomain(), nil
}

// GetByUsername retrieves a user by username.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	query := `
		SELECT user_id, username, email, password_hash, is_active, is_locked,
			failed_login_attempts, locked_until, two_factor_enabled, two_factor_secret,
			last_login_at, last_login_ip, password_changed_at,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_user
		WHERE username = $1 AND deleted_at IS NULL
	`

	var u userRow
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsActive, &u.IsLocked,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.TwoFactorEnabled, &u.TwoFactorSecret,
		&u.LastLoginAt, &u.LastLoginIP, &u.PasswordChangedAt,
		&u.CreatedAt, &u.CreatedBy, &u.UpdatedAt, &u.UpdatedBy, &u.DeletedAt, &u.DeletedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by username: %w", err)
	}

	return u.toDomain(), nil
}

// GetByEmail retrieves a user by email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	query := `
		SELECT user_id, username, email, password_hash, is_active, is_locked,
			failed_login_attempts, locked_until, two_factor_enabled, two_factor_secret,
			last_login_at, last_login_ip, password_changed_at,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_user
		WHERE email = $1 AND deleted_at IS NULL
	`

	var u userRow
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsActive, &u.IsLocked,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.TwoFactorEnabled, &u.TwoFactorSecret,
		&u.LastLoginAt, &u.LastLoginIP, &u.PasswordChangedAt,
		&u.CreatedAt, &u.CreatedBy, &u.UpdatedAt, &u.UpdatedBy, &u.DeletedAt, &u.DeletedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return u.toDomain(), nil
}

// Update updates a user.
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	query := `
		UPDATE mst_user SET
			email = $2, password_hash = $3, is_active = $4, is_locked = $5,
			failed_login_attempts = $6, locked_until = $7, two_factor_enabled = $8,
			two_factor_secret = $9, last_login_at = $10, last_login_ip = $11,
			password_changed_at = $12, updated_at = $13, updated_by = $14
		WHERE user_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		u.ID(), u.Email(), u.PasswordHash(), u.IsActive(), u.IsLocked(),
		u.FailedLoginAttempts(), u.LockedUntil(), u.TwoFactorEnabled(),
		u.TwoFactorSecret(), u.LastLoginAt(), u.LastLoginIP(),
		u.PasswordChangedAt(), u.Audit().UpdatedAt, u.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
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

// Delete soft-deletes a user.
func (r *UserRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE mst_user SET
			is_active = false, deleted_at = $2, deleted_by = $3
		WHERE user_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
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

// GetDetailByUserID retrieves user detail by user ID.
func (r *UserRepository) GetDetailByUserID(ctx context.Context, userID uuid.UUID) (*user.Detail, error) {
	query := `
		SELECT detail_id, user_id, section_id, employee_code, full_name, first_name, last_name,
			phone, profile_picture, position, date_of_birth, address, extra_data,
			created_at, created_by, updated_at, updated_by
		FROM mst_user_detail
		WHERE user_id = $1
	`

	var d userDetailRow
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&d.ID, &d.UserID, &d.SectionID, &d.EmployeeCode, &d.FullName, &d.FirstName, &d.LastName,
		&d.Phone, &d.ProfilePicture, &d.Position, &d.DateOfBirth, &d.Address, &d.ExtraData,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get user detail: %w", err)
	}

	return d.toDomain(), nil
}

// UpdateDetail updates user detail.
func (r *UserRepository) UpdateDetail(ctx context.Context, detail *user.Detail) error {
	extraDataJSON, err := json.Marshal(detail.ExtraData())
	if err != nil {
		return fmt.Errorf("failed to marshal extra data: %w", err)
	}
	query := `
		UPDATE mst_user_detail SET
			section_id = $2, full_name = $3, first_name = $4, last_name = $5,
			phone = $6, profile_picture = $7, position = $8, date_of_birth = $9,
			address = $10, extra_data = $11, updated_at = $12, updated_by = $13
		WHERE user_id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		detail.UserID(), detail.SectionID(), detail.FullName(), detail.FirstName(), detail.LastName(),
		detail.Phone(), detail.ProfilePicture(), detail.Position(), detail.DateOfBirth(),
		detail.Address(), extraDataJSON, detail.Audit().UpdatedAt, detail.Audit().UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to update user detail: %w", err)
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

// List lists users with pagination and filtering.
func (r *UserRepository) List(ctx context.Context, params user.ListParams) ([]*user.User, int64, error) {
	var conditions []string
	var args []interface{}
	argPos := 1

	conditions = append(conditions, "deleted_at IS NULL")

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(username ILIKE $%d OR email ILIKE $%d)", argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}

	if params.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argPos))
		args = append(args, *params.IsActive)
		argPos++
	}

	whereClause := strings.Join(conditions, " AND ")

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_user WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Build query
	sortBy := "created_at"
	if params.SortBy != "" {
		sortBy = params.SortBy
	}
	sortOrder := "DESC"
	if params.SortOrder != "" && (params.SortOrder == "ASC" || params.SortOrder == "asc") {
		sortOrder = "ASC"
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT user_id, username, email, password_hash, is_active, is_locked,
			failed_login_attempts, locked_until, two_factor_enabled, two_factor_secret,
			last_login_at, last_login_ip, password_changed_at,
			created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_user
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortBy, sortOrder, argPos, argPos+1)

	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in user list")
		}
	}()

	var users []*user.User
	for rows.Next() {
		var u userRow
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsActive, &u.IsLocked,
			&u.FailedLoginAttempts, &u.LockedUntil, &u.TwoFactorEnabled, &u.TwoFactorSecret,
			&u.LastLoginAt, &u.LastLoginIP, &u.PasswordChangedAt,
			&u.CreatedAt, &u.CreatedBy, &u.UpdatedAt, &u.UpdatedBy, &u.DeletedAt, &u.DeletedBy,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u.toDomain())
	}

	return users, total, nil
}

// ListWithDetails lists users with their details.
func (r *UserRepository) ListWithDetails(ctx context.Context, params user.ListParams) ([]*user.WithDetail, int64, error) {
	// Implementation similar to List but with JOIN to user_detail
	// For brevity, we'll reuse the List method and fetch details separately
	users, total, err := r.List(ctx, params)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*user.WithDetail, len(users))
	for i, u := range users {
		detail, err := r.GetDetailByUserID(ctx, u.ID())
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			log.Warn().Err(err).Str("user_id", u.ID().String()).Msg("failed to get user detail")
		}
		roles, err := r.getUserRoles(ctx, u.ID())
		if err != nil {
			log.Warn().Err(err).Str("user_id", u.ID().String()).Msg("failed to get user roles")
		}

		result[i] = &user.WithDetail{
			User:   u,
			Detail: detail,
			Roles:  roles,
		}
	}

	return result, total, nil
}

func (r *UserRepository) getUserRoles(ctx context.Context, userID uuid.UUID) ([]user.RoleInfo, error) {
	query := `
		SELECT r.role_id, r.role_code, r.role_name
		FROM mst_role r
		INNER JOIN user_roles ur ON r.role_id = ur.role_id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in getUserRoles")
		}
	}()

	var roles []user.RoleInfo
	for rows.Next() {
		var role user.RoleInfo
		if err := rows.Scan(&role.RoleID, &role.RoleCode, &role.RoleName); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, nil
}

// ExistsByUsername checks if a username exists.
func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_user WHERE username = $1 AND deleted_at IS NULL)"
	if err := r.db.QueryRowContext(ctx, query, username).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// ExistsByEmail checks if an email exists.
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_user WHERE email = $1 AND deleted_at IS NULL)"
	if err := r.db.QueryRowContext(ctx, query, email).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// ExistsByEmployeeCode checks if an employee code exists.
func (r *UserRepository) ExistsByEmployeeCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_user_detail WHERE employee_code = $1)"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// GetRolesAndPermissions retrieves roles and direct permissions for a user.
func (r *UserRepository) GetRolesAndPermissions(ctx context.Context, userID uuid.UUID) ([]user.RoleRef, []user.PermissionRef, error) {
	// Get roles
	roleQuery := `
		SELECT r.role_id, r.role_code, r.role_name
		FROM mst_role r
		INNER JOIN user_roles ur ON r.role_id = ur.role_id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL
	`

	roleRows, err := r.db.QueryContext(ctx, roleQuery, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	defer func() {
		if err := roleRows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close roleRows in GetRolesAndPermissions")
		}
	}()

	var roles []user.RoleRef
	for roleRows.Next() {
		var id uuid.UUID
		var code, name string
		if err := roleRows.Scan(&id, &code, &name); err != nil {
			return nil, nil, err
		}
		roles = append(roles, &roleRefImpl{id: id, code: code, name: name})
	}

	// Get direct permissions
	permQuery := `
		SELECT p.permission_id, p.permission_code
		FROM mst_permission p
		INNER JOIN user_permissions up ON p.permission_id = up.permission_id
		WHERE up.user_id = $1 AND p.is_active = true
	`

	permRows, err := r.db.QueryContext(ctx, permQuery, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user permissions: %w", err)
	}
	defer func() {
		if err := permRows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close permRows in GetRolesAndPermissions")
		}
	}()

	var permissions []user.PermissionRef
	for permRows.Next() {
		var id uuid.UUID
		var code string
		if err := permRows.Scan(&id, &code); err != nil {
			return nil, nil, err
		}
		permissions = append(permissions, &permissionRefImpl{id: id, code: code})
	}

	return roles, permissions, nil
}

// roleRefImpl implements user.RoleRef interface.
type roleRefImpl struct {
	id   uuid.UUID
	code string
	name string
}

func (r *roleRefImpl) ID() uuid.UUID { return r.id }
func (r *roleRefImpl) Code() string  { return r.code }
func (r *roleRefImpl) Name() string  { return r.name }

// permissionRefImpl implements user.PermissionRef interface.
type permissionRefImpl struct {
	id   uuid.UUID
	code string
}

func (p *permissionRefImpl) ID() uuid.UUID { return p.id }
func (p *permissionRefImpl) Code() string  { return p.code }

// BatchCreate creates multiple users in a batch.
func (r *UserRepository) BatchCreate(ctx context.Context, users []*user.User, details []*user.Detail) (int, error) {
	count := 0
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for i, u := range users {
			var detail *user.Detail
			if i < len(details) {
				detail = details[i]
			}

			// Insert user
			query := `
				INSERT INTO mst_user (
					user_id, username, email, password_hash, is_active, is_locked,
					failed_login_attempts, locked_until, two_factor_enabled, two_factor_secret,
					last_login_at, last_login_ip, password_changed_at,
					created_at, created_by
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
			`
			_, err := tx.ExecContext(ctx, query,
				u.ID(), u.Username(), u.Email(), u.PasswordHash(), u.IsActive(), u.IsLocked(),
				u.FailedLoginAttempts(), u.LockedUntil(), u.TwoFactorEnabled(), u.TwoFactorSecret(),
				u.LastLoginAt(), u.LastLoginIP(), u.PasswordChangedAt(),
				u.Audit().CreatedAt, u.Audit().CreatedBy,
			)
			if err != nil {
				return fmt.Errorf("failed to insert user %s: %w", u.Username(), err)
			}

			if detail != nil {
				extraDataJSON, err := json.Marshal(detail.ExtraData())
				if err != nil {
					return fmt.Errorf("failed to marshal extra data for %s: %w", u.Username(), err)
				}
				query = `
					INSERT INTO mst_user_detail (
						detail_id, user_id, section_id, employee_code, full_name, first_name, last_name,
						phone, profile_picture, position, date_of_birth, address, extra_data,
						created_at, created_by
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
				`
				_, err = tx.ExecContext(ctx, query,
					detail.ID(), detail.UserID(), detail.SectionID(), detail.EmployeeCode(),
					detail.FullName(), detail.FirstName(), detail.LastName(),
					detail.Phone(), detail.ProfilePicture(), detail.Position(),
					detail.DateOfBirth(), detail.Address(), extraDataJSON,
					detail.Audit().CreatedAt, detail.Audit().CreatedBy,
				)
				if err != nil {
					return fmt.Errorf("failed to insert user detail for %s: %w", u.Username(), err)
				}
			}

			count++
		}
		return nil
	})

	return count, err
}

// Helper structs for scanning
type userRow struct {
	ID                  uuid.UUID
	Username            string
	Email               string
	PasswordHash        string
	IsActive            bool
	IsLocked            bool
	FailedLoginAttempts int
	LockedUntil         *time.Time
	TwoFactorEnabled    bool
	TwoFactorSecret     sql.NullString
	LastLoginAt         *time.Time
	LastLoginIP         sql.NullString
	PasswordChangedAt   *time.Time
	CreatedAt           time.Time
	CreatedBy           string
	UpdatedAt           *time.Time
	UpdatedBy           *string
	DeletedAt           *time.Time
	DeletedBy           *string
}

func (r *userRow) toDomain() *user.User {
	var secret, ip string
	if r.TwoFactorSecret.Valid {
		secret = r.TwoFactorSecret.String
	}
	if r.LastLoginIP.Valid {
		ip = r.LastLoginIP.String
	}

	audit := shared.AuditInfo{
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
		DeletedAt: r.DeletedAt,
		DeletedBy: r.DeletedBy,
	}

	return user.ReconstructUser(
		r.ID, r.Username, r.Email, r.PasswordHash,
		r.IsActive, r.IsLocked, r.FailedLoginAttempts, r.LockedUntil,
		r.TwoFactorEnabled, secret, r.LastLoginAt, ip, r.PasswordChangedAt,
		audit,
	)
}

type userDetailRow struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	SectionID      *uuid.UUID
	EmployeeCode   string
	FullName       string
	FirstName      sql.NullString
	LastName       sql.NullString
	Phone          sql.NullString
	ProfilePicture sql.NullString
	Position       sql.NullString
	DateOfBirth    *time.Time
	Address        sql.NullString
	ExtraData      []byte
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      *time.Time
	UpdatedBy      *string
}

func (r *userDetailRow) toDomain() *user.Detail {
	var extraData map[string]interface{}
	if len(r.ExtraData) > 0 {
		if err := json.Unmarshal(r.ExtraData, &extraData); err != nil {
			log.Warn().Err(err).Str("user_id", r.UserID.String()).Msg("failed to unmarshal user detail extra data")
		}
	}

	audit := shared.AuditInfo{
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
	}

	return user.ReconstructDetail(
		r.ID, r.UserID, r.SectionID, r.EmployeeCode, r.FullName,
		r.FirstName.String, r.LastName.String,
		r.Phone.String, r.ProfilePicture.String, r.Position.String,
		r.DateOfBirth, r.Address.String, extraData,
		audit,
	)
}
