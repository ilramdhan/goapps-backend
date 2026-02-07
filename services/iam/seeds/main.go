// Package main seeds default IAM data into the database.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

const (
	systemUser        = "system-seed"
	defaultBcryptCost = bcrypt.DefaultCost
)

// role defines a role to seed.
type role struct {
	Code        string
	Name        string
	Description string
	IsSystem    bool
}

// permission defines a permission to seed.
type permission struct {
	Code        string
	Name        string
	ServiceName string
	ModuleName  string
	ActionType  string
}

// defaultRoles returns the roles to seed.
func defaultRoles() []role {
	return []role{
		{Code: "SUPER_ADMIN", Name: "Super Administrator", Description: "Full system access with all permissions", IsSystem: true},
		{Code: "ADMIN", Name: "Administrator", Description: "Administrative access for managing users and settings", IsSystem: true},
		{Code: "USER", Name: "Regular User", Description: "Standard user with create and edit access", IsSystem: false},
		{Code: "VIEWER", Name: "View Only", Description: "Read-only access to all resources", IsSystem: false},
	}
}

// defaultPermissions returns the permissions to seed.
func defaultPermissions() []permission {
	perms := []permission{
		// IAM - User management
		{Code: "iam.user.user.view", Name: "View Users", ServiceName: "iam", ModuleName: "user", ActionType: "view"},
		{Code: "iam.user.user.create", Name: "Create User", ServiceName: "iam", ModuleName: "user", ActionType: "create"},
		{Code: "iam.user.user.update", Name: "Update User", ServiceName: "iam", ModuleName: "user", ActionType: "update"},
		{Code: "iam.user.user.delete", Name: "Delete User", ServiceName: "iam", ModuleName: "user", ActionType: "delete"},
		{Code: "iam.user.user.export", Name: "Export Users", ServiceName: "iam", ModuleName: "user", ActionType: "export"},
		{Code: "iam.user.user.import", Name: "Import Users", ServiceName: "iam", ModuleName: "user", ActionType: "import"},

		// IAM - Role management
		{Code: "iam.role.role.view", Name: "View Roles", ServiceName: "iam", ModuleName: "role", ActionType: "view"},
		{Code: "iam.role.role.create", Name: "Create Role", ServiceName: "iam", ModuleName: "role", ActionType: "create"},
		{Code: "iam.role.role.update", Name: "Update Role", ServiceName: "iam", ModuleName: "role", ActionType: "update"},
		{Code: "iam.role.role.delete", Name: "Delete Role", ServiceName: "iam", ModuleName: "role", ActionType: "delete"},

		// IAM - Permission management
		{Code: "iam.role.permission.view", Name: "View Permissions", ServiceName: "iam", ModuleName: "role", ActionType: "view"},
		{Code: "iam.role.permission.create", Name: "Create Permission", ServiceName: "iam", ModuleName: "role", ActionType: "create"},
		{Code: "iam.role.permission.update", Name: "Update Permission", ServiceName: "iam", ModuleName: "role", ActionType: "update"},
		{Code: "iam.role.permission.delete", Name: "Delete Permission", ServiceName: "iam", ModuleName: "role", ActionType: "delete"},

		// IAM - Menu management
		{Code: "iam.menu.menu.view", Name: "View Menus", ServiceName: "iam", ModuleName: "menu", ActionType: "view"},
		{Code: "iam.menu.menu.create", Name: "Create Menu", ServiceName: "iam", ModuleName: "menu", ActionType: "create"},
		{Code: "iam.menu.menu.update", Name: "Update Menu", ServiceName: "iam", ModuleName: "menu", ActionType: "update"},
		{Code: "iam.menu.menu.delete", Name: "Delete Menu", ServiceName: "iam", ModuleName: "menu", ActionType: "delete"},

		// IAM - Organization: Company
		{Code: "iam.organization.company.view", Name: "View Companies", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.company.create", Name: "Create Company", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.company.update", Name: "Update Company", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.company.delete", Name: "Delete Company", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},

		// IAM - Organization: Division
		{Code: "iam.organization.division.view", Name: "View Divisions", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.division.create", Name: "Create Division", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.division.update", Name: "Update Division", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.division.delete", Name: "Delete Division", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},

		// IAM - Organization: Department
		{Code: "iam.organization.department.view", Name: "View Departments", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.department.create", Name: "Create Department", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.department.update", Name: "Update Department", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.department.delete", Name: "Delete Department", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},

		// IAM - Organization: Section
		{Code: "iam.organization.section.view", Name: "View Sections", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.section.create", Name: "Create Section", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.section.update", Name: "Update Section", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.section.delete", Name: "Delete Section", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},

		// IAM - Audit
		{Code: "iam.audit.audit.view", Name: "View Audit Logs", ServiceName: "iam", ModuleName: "audit", ActionType: "view"},

		// IAM - Session
		{Code: "iam.session.session.view", Name: "View Sessions", ServiceName: "iam", ModuleName: "session", ActionType: "view"},
		{Code: "iam.session.session.delete", Name: "Delete Session", ServiceName: "iam", ModuleName: "session", ActionType: "delete"},

		// Finance - UOM
		{Code: "finance.master.uom.view", Name: "View UOMs", ServiceName: "finance", ModuleName: "master", ActionType: "view"},
		{Code: "finance.master.uom.create", Name: "Create UOM", ServiceName: "finance", ModuleName: "master", ActionType: "create"},
		{Code: "finance.master.uom.update", Name: "Update UOM", ServiceName: "finance", ModuleName: "master", ActionType: "update"},
		{Code: "finance.master.uom.delete", Name: "Delete UOM", ServiceName: "finance", ModuleName: "master", ActionType: "delete"},
		{Code: "finance.master.uom.export", Name: "Export UOMs", ServiceName: "finance", ModuleName: "master", ActionType: "export"},
		{Code: "finance.master.uom.import", Name: "Import UOMs", ServiceName: "finance", ModuleName: "master", ActionType: "import"},
	}

	return perms
}

func main() {
	// Setup logger
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	log.Info().Msg("Starting IAM database seeding...")

	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Seeding failed")
	}

	log.Info().Msg("Seeding completed successfully")
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Connect to database
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close database connection")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Run all seeds in a transaction
	return db.Transaction(ctx, func(tx *sql.Tx) error {
		// 1. Seed roles
		roleIDs, err := seedRoles(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to seed roles: %w", err)
		}

		// 2. Seed permissions
		permIDs, err := seedPermissions(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to seed permissions: %w", err)
		}

		// 3. Seed role-permission assignments
		if err := seedRolePermissions(ctx, tx, roleIDs, permIDs); err != nil {
			return fmt.Errorf("failed to seed role permissions: %w", err)
		}

		// 4. Seed admin user
		adminUserID, err := seedAdminUser(ctx, tx)
		if err != nil {
			return fmt.Errorf("failed to seed admin user: %w", err)
		}

		// 5. Seed user-role assignment
		if err := seedUserRoles(ctx, tx, adminUserID, roleIDs); err != nil {
			return fmt.Errorf("failed to seed user roles: %w", err)
		}

		return nil
	})
}

// seedRoles inserts default roles and returns a map of role_code -> role_id.
func seedRoles(ctx context.Context, tx *sql.Tx) (map[string]uuid.UUID, error) {
	log.Info().Msg("Seeding roles...")

	roles := defaultRoles()
	roleIDs := make(map[string]uuid.UUID, len(roles))

	for _, r := range roles {
		id := uuid.New()
		_, err := tx.ExecContext(ctx, `
			INSERT INTO mst_role (role_id, role_code, role_name, description, is_system, is_active, created_by)
			VALUES ($1, $2, $3, $4, $5, true, $6)
			ON CONFLICT (role_code) DO NOTHING`,
			id, r.Code, r.Name, r.Description, r.IsSystem, systemUser,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert role %s: %w", r.Code, err)
		}

		// Retrieve the actual ID (may differ if row already existed)
		var actualID uuid.UUID
		err = tx.QueryRowContext(ctx, `SELECT role_id FROM mst_role WHERE role_code = $1`, r.Code).Scan(&actualID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve role_id for %s: %w", r.Code, err)
		}
		roleIDs[r.Code] = actualID

		log.Info().Str("role_code", r.Code).Str("role_id", actualID.String()).Msg("Role seeded")
	}

	return roleIDs, nil
}

// seedPermissions inserts default permissions and returns a map of permission_code -> permission_id.
func seedPermissions(ctx context.Context, tx *sql.Tx) (map[string]uuid.UUID, error) {
	log.Info().Msg("Seeding permissions...")

	perms := defaultPermissions()
	permIDs := make(map[string]uuid.UUID, len(perms))

	for _, p := range perms {
		id := uuid.New()
		_, err := tx.ExecContext(ctx, `
			INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8)
			ON CONFLICT (permission_code) DO NOTHING`,
			id, p.Code, p.Name, p.Name, p.ServiceName, p.ModuleName, p.ActionType, systemUser,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert permission %s: %w", p.Code, err)
		}

		// Retrieve the actual ID (may differ if row already existed)
		var actualID uuid.UUID
		err = tx.QueryRowContext(ctx, `SELECT permission_id FROM mst_permission WHERE permission_code = $1`, p.Code).Scan(&actualID)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve permission_id for %s: %w", p.Code, err)
		}
		permIDs[p.Code] = actualID

		log.Debug().Str("permission_code", p.Code).Msg("Permission seeded")
	}

	log.Info().Int("count", len(permIDs)).Msg("Permissions seeded")
	return permIDs, nil
}

// seedRolePermissions assigns permissions to roles.
func seedRolePermissions(ctx context.Context, tx *sql.Tx, roleIDs map[string]uuid.UUID, permIDs map[string]uuid.UUID) error { //nolint:gocyclo,gocognit // seed function with inherent complexity from role-permission matrix
	log.Info().Msg("Seeding role-permission assignments...")

	allPermCodes := make([]string, 0, len(permIDs))
	for code := range permIDs {
		allPermCodes = append(allPermCodes, code)
	}

	// SUPER_ADMIN gets all permissions
	superAdminPerms := allPermCodes

	// ADMIN gets all permissions except delete on role and permission
	adminExcluded := map[string]bool{
		"iam.role.role.delete":       true,
		"iam.role.permission.delete": true,
	}
	adminPerms := make([]string, 0, len(allPermCodes))
	for _, code := range allPermCodes {
		if !adminExcluded[code] {
			adminPerms = append(adminPerms, code)
		}
	}

	// USER gets all view permissions + create/update on non-admin resources
	// Non-admin resources: everything except role and permission management
	adminResources := map[string]bool{
		"iam.role.role":       true,
		"iam.role.permission": true,
	}
	userPerms := make([]string, 0)
	for _, code := range allPermCodes {
		parts := strings.Split(code, ".")
		if len(parts) != 4 {
			continue
		}
		action := parts[3]
		resource := strings.Join(parts[:3], ".")

		if action == "view" {
			userPerms = append(userPerms, code)
		} else if (action == "create" || action == "update" || action == "export" || action == "import") && !adminResources[resource] {
			userPerms = append(userPerms, code)
		}
	}

	// VIEWER gets only view permissions
	viewerPerms := make([]string, 0)
	for _, code := range allPermCodes {
		if strings.HasSuffix(code, ".view") {
			viewerPerms = append(viewerPerms, code)
		}
	}

	assignments := map[string][]string{
		"SUPER_ADMIN": superAdminPerms,
		"ADMIN":       adminPerms,
		"USER":        userPerms,
		"VIEWER":      viewerPerms,
	}

	totalAssigned := 0
	for roleCode, permCodes := range assignments {
		roleID, ok := roleIDs[roleCode]
		if !ok {
			return fmt.Errorf("role %s not found", roleCode)
		}

		for _, permCode := range permCodes {
			permID, ok := permIDs[permCode]
			if !ok {
				return fmt.Errorf("permission %s not found", permCode)
			}

			_, err := tx.ExecContext(ctx, `
				INSERT INTO role_permissions (id, role_id, permission_id, assigned_by)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (role_id, permission_id) DO NOTHING`,
				uuid.New(), roleID, permID, systemUser,
			)
			if err != nil {
				return fmt.Errorf("failed to assign permission %s to role %s: %w", permCode, roleCode, err)
			}
			totalAssigned++
		}

		log.Info().Str("role", roleCode).Int("permissions", len(permCodes)).Msg("Role permissions assigned")
	}

	log.Info().Int("total_assignments", totalAssigned).Msg("Role-permission assignments seeded")
	return nil
}

// seedAdminUser creates the default admin user and returns the user_id.
func seedAdminUser(ctx context.Context, tx *sql.Tx) (uuid.UUID, error) {
	log.Info().Msg("Seeding admin user...")

	// Hash the default password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("admin123"), defaultBcryptCost)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to hash password: %w", err)
	}

	userID := uuid.New()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO mst_user (user_id, username, email, password_hash, is_active, created_by)
		VALUES ($1, $2, $3, $4, true, $5)
		ON CONFLICT (username) DO NOTHING`,
		userID, "admin", "admin@goapps.local", string(passwordHash), systemUser,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert admin user: %w", err)
	}

	// Retrieve the actual ID (may differ if row already existed)
	var actualID uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM mst_user WHERE username = $1`, "admin").Scan(&actualID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to retrieve admin user_id: %w", err)
	}

	// Seed admin user detail
	_, err = tx.ExecContext(ctx, `
		INSERT INTO mst_user_detail (detail_id, user_id, employee_code, full_name, first_name, last_name, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO NOTHING`,
		uuid.New(), actualID, "EMP-000", "System Administrator", "System", "Administrator", systemUser,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert admin user detail: %w", err)
	}

	log.Info().Str("user_id", actualID.String()).Str("username", "admin").Msg("Admin user seeded")
	return actualID, nil
}

// seedUserRoles assigns the SUPER_ADMIN role to the admin user.
func seedUserRoles(ctx context.Context, tx *sql.Tx, userID uuid.UUID, roleIDs map[string]uuid.UUID) error {
	log.Info().Msg("Seeding user-role assignments...")

	superAdminRoleID, ok := roleIDs["SUPER_ADMIN"]
	if !ok {
		return fmt.Errorf("SUPER_ADMIN role not found")
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO user_roles (id, user_id, role_id, assigned_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, role_id) DO NOTHING`,
		uuid.New(), userID, superAdminRoleID, systemUser,
	)
	if err != nil {
		return fmt.Errorf("failed to assign SUPER_ADMIN role to admin user: %w", err)
	}

	log.Info().
		Str("user_id", userID.String()).
		Str("role", "SUPER_ADMIN").
		Msg("User-role assignment seeded")
	return nil
}
