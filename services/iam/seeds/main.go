// Package main seeds default IAM data into the database.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/password"
	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/postgres"
)

// adminCredentials holds the seed admin user configuration.
// Values are read from environment variables to avoid hardcoded credentials
// in source code. Defaults are only suitable for local development.
type adminCredentials struct {
	Username string
	Email    string
	Password string
}

// getAdminCredentials reads admin credentials from environment variables.
// If SEED_ADMIN_PASSWORD is not set in non-development environments,
// a random password is generated and printed to stdout.
func getAdminCredentials(appEnv string) adminCredentials {
	creds := adminCredentials{
		Username: envOrDefault("SEED_ADMIN_USERNAME", "admin"),
		Email:    envOrDefault("SEED_ADMIN_EMAIL", "admin@goapps.local"),
		Password: os.Getenv("SEED_ADMIN_PASSWORD"),
	}

	if creds.Password == "" {
		if appEnv == "development" || appEnv == "" {
			// Local dev: use a known default
			creds.Password = "admin123" //nolint:gosec // intentional default for local dev only
		} else {
			// Staging/production: generate a random password and print it
			creds.Password = generateRandomPassword(24)
			log.Warn().
				Str("env", appEnv).
				Msg("SEED_ADMIN_PASSWORD not set — generated random password (printed below, save it now!)")
			fmt.Printf("\n========================================\n")
			fmt.Printf("  GENERATED ADMIN PASSWORD: %s\n", creds.Password)
			fmt.Printf("  USERNAME: %s\n", creds.Username)
			fmt.Printf("  EMAIL: %s\n", creds.Email)
			fmt.Printf("  ⚠ Save this password now — it will NOT be shown again.\n")
			fmt.Printf("========================================\n\n")
		}
	}

	return creds
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func generateRandomPassword(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to uuid if crypto/rand fails (extremely unlikely)
		return uuid.New().String()
	}
	return hex.EncodeToString(bytes)[:length]
}

const (
	systemUser = "system-seed"
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
// Permission codes MUST match the codes used in auth/permission interceptors.
func defaultPermissions() []permission {
	perms := []permission{
		// IAM - User Account management
		{Code: "iam.user.account.view", Name: "View Users", ServiceName: "iam", ModuleName: "user", ActionType: "view"},
		{Code: "iam.user.account.create", Name: "Create User", ServiceName: "iam", ModuleName: "user", ActionType: "create"},
		{Code: "iam.user.account.update", Name: "Update User", ServiceName: "iam", ModuleName: "user", ActionType: "update"},
		{Code: "iam.user.account.delete", Name: "Delete User", ServiceName: "iam", ModuleName: "user", ActionType: "delete"},
		{Code: "iam.user.account.export", Name: "Export Users", ServiceName: "iam", ModuleName: "user", ActionType: "export"},
		{Code: "iam.user.account.import", Name: "Import Users", ServiceName: "iam", ModuleName: "user", ActionType: "import"},

		// IAM - RBAC Role management
		{Code: "iam.rbac.role.view", Name: "View Roles", ServiceName: "iam", ModuleName: "rbac", ActionType: "view"},
		{Code: "iam.rbac.role.create", Name: "Create Role", ServiceName: "iam", ModuleName: "rbac", ActionType: "create"},
		{Code: "iam.rbac.role.update", Name: "Update Role", ServiceName: "iam", ModuleName: "rbac", ActionType: "update"},
		{Code: "iam.rbac.role.delete", Name: "Delete Role", ServiceName: "iam", ModuleName: "rbac", ActionType: "delete"},
		{Code: "iam.rbac.role.export", Name: "Export Roles", ServiceName: "iam", ModuleName: "rbac", ActionType: "export"},
		{Code: "iam.rbac.role.import", Name: "Import Roles", ServiceName: "iam", ModuleName: "rbac", ActionType: "import"},

		// IAM - RBAC Permission management
		{Code: "iam.rbac.permission.view", Name: "View Permissions", ServiceName: "iam", ModuleName: "rbac", ActionType: "view"},
		{Code: "iam.rbac.permission.create", Name: "Create Permission", ServiceName: "iam", ModuleName: "rbac", ActionType: "create"},
		{Code: "iam.rbac.permission.update", Name: "Update Permission", ServiceName: "iam", ModuleName: "rbac", ActionType: "update"},
		{Code: "iam.rbac.permission.delete", Name: "Delete Permission", ServiceName: "iam", ModuleName: "rbac", ActionType: "delete"},
		{Code: "iam.rbac.permission.export", Name: "Export Permissions", ServiceName: "iam", ModuleName: "rbac", ActionType: "export"},
		{Code: "iam.rbac.permission.import", Name: "Import Permissions", ServiceName: "iam", ModuleName: "rbac", ActionType: "import"},

		// IAM - Menu management
		{Code: "iam.menu.menu.view", Name: "View Menus", ServiceName: "iam", ModuleName: "menu", ActionType: "view"},
		{Code: "iam.menu.menu.create", Name: "Create Menu", ServiceName: "iam", ModuleName: "menu", ActionType: "create"},
		{Code: "iam.menu.menu.update", Name: "Update Menu", ServiceName: "iam", ModuleName: "menu", ActionType: "update"},
		{Code: "iam.menu.menu.delete", Name: "Delete Menu", ServiceName: "iam", ModuleName: "menu", ActionType: "delete"},
		{Code: "iam.menu.menu.export", Name: "Export Menus", ServiceName: "iam", ModuleName: "menu", ActionType: "export"},
		{Code: "iam.menu.menu.import", Name: "Import Menus", ServiceName: "iam", ModuleName: "menu", ActionType: "import"},

		// IAM - Organization: Company
		{Code: "iam.organization.company.view", Name: "View Companies", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.company.create", Name: "Create Company", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.company.update", Name: "Update Company", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.company.delete", Name: "Delete Company", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},
		{Code: "iam.organization.company.export", Name: "Export Companies", ServiceName: "iam", ModuleName: "organization", ActionType: "export"},
		{Code: "iam.organization.company.import", Name: "Import Companies", ServiceName: "iam", ModuleName: "organization", ActionType: "import"},

		// IAM - Organization: Division
		{Code: "iam.organization.division.view", Name: "View Divisions", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.division.create", Name: "Create Division", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.division.update", Name: "Update Division", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.division.delete", Name: "Delete Division", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},
		{Code: "iam.organization.division.export", Name: "Export Divisions", ServiceName: "iam", ModuleName: "organization", ActionType: "export"},
		{Code: "iam.organization.division.import", Name: "Import Divisions", ServiceName: "iam", ModuleName: "organization", ActionType: "import"},

		// IAM - Organization: Department
		{Code: "iam.organization.department.view", Name: "View Departments", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.department.create", Name: "Create Department", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.department.update", Name: "Update Department", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.department.delete", Name: "Delete Department", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},
		{Code: "iam.organization.department.export", Name: "Export Departments", ServiceName: "iam", ModuleName: "organization", ActionType: "export"},
		{Code: "iam.organization.department.import", Name: "Import Departments", ServiceName: "iam", ModuleName: "organization", ActionType: "import"},

		// IAM - Organization: Section
		{Code: "iam.organization.section.view", Name: "View Sections", ServiceName: "iam", ModuleName: "organization", ActionType: "view"},
		{Code: "iam.organization.section.create", Name: "Create Section", ServiceName: "iam", ModuleName: "organization", ActionType: "create"},
		{Code: "iam.organization.section.update", Name: "Update Section", ServiceName: "iam", ModuleName: "organization", ActionType: "update"},
		{Code: "iam.organization.section.delete", Name: "Delete Section", ServiceName: "iam", ModuleName: "organization", ActionType: "delete"},
		{Code: "iam.organization.section.export", Name: "Export Sections", ServiceName: "iam", ModuleName: "organization", ActionType: "export"},
		{Code: "iam.organization.section.import", Name: "Import Sections", ServiceName: "iam", ModuleName: "organization", ActionType: "import"},

		// IAM - Audit
		{Code: "iam.audit.log.view", Name: "View Audit Logs", ServiceName: "iam", ModuleName: "audit", ActionType: "view"},
		{Code: "iam.audit.log.export", Name: "Export Audit Logs", ServiceName: "iam", ModuleName: "audit", ActionType: "export"},

		// IAM - Session
		{Code: "iam.session.session.view", Name: "View Sessions", ServiceName: "iam", ModuleName: "session", ActionType: "view"},
		{Code: "iam.session.session.delete", Name: "Delete Session", ServiceName: "iam", ModuleName: "session", ActionType: "delete"},

		// Finance — module-level permissions (used by menu visibility)
		// Format: service.module.entity.action (4 parts required by DB constraint)
		{Code: "finance.module.root.view", Name: "Access Finance Module", ServiceName: "finance", ModuleName: "module", ActionType: "view"},
		{Code: "finance.module.dashboard.view", Name: "View Finance Dashboard", ServiceName: "finance", ModuleName: "module", ActionType: "view"},
		{Code: "finance.module.master.view", Name: "Access Finance Master", ServiceName: "finance", ModuleName: "module", ActionType: "view"},
		{Code: "finance.module.transaction.view", Name: "Access Finance Transaction", ServiceName: "finance", ModuleName: "module", ActionType: "view"},

		// Finance - UOM
		{Code: "finance.master.uom.view", Name: "View UOMs", ServiceName: "finance", ModuleName: "master", ActionType: "view"},
		{Code: "finance.master.uom.create", Name: "Create UOM", ServiceName: "finance", ModuleName: "master", ActionType: "create"},
		{Code: "finance.master.uom.update", Name: "Update UOM", ServiceName: "finance", ModuleName: "master", ActionType: "update"},
		{Code: "finance.master.uom.delete", Name: "Delete UOM", ServiceName: "finance", ModuleName: "master", ActionType: "delete"},
		{Code: "finance.master.uom.export", Name: "Export UOMs", ServiceName: "finance", ModuleName: "master", ActionType: "export"},
		{Code: "finance.master.uom.import", Name: "Import UOMs", ServiceName: "finance", ModuleName: "master", ActionType: "import"},

		// Finance - Parameters
		{Code: "finance.master.parameters.view", Name: "View Parameters", ServiceName: "finance", ModuleName: "master", ActionType: "view"},
		{Code: "finance.master.parameters.create", Name: "Create Parameter", ServiceName: "finance", ModuleName: "master", ActionType: "create"},
		{Code: "finance.master.parameters.update", Name: "Update Parameter", ServiceName: "finance", ModuleName: "master", ActionType: "update"},
		{Code: "finance.master.parameters.delete", Name: "Delete Parameter", ServiceName: "finance", ModuleName: "master", ActionType: "delete"},

		// Finance - Costing Process
		{Code: "finance.transaction.costing.view", Name: "View Costing Process", ServiceName: "finance", ModuleName: "transaction", ActionType: "view"},
		{Code: "finance.transaction.costing.create", Name: "Create Costing Process", ServiceName: "finance", ModuleName: "transaction", ActionType: "create"},
		{Code: "finance.transaction.costing.update", Name: "Update Costing Process", ServiceName: "finance", ModuleName: "transaction", ActionType: "update"},

		// IT — module-level permissions
		{Code: "it.module.root.view", Name: "Access IT Module", ServiceName: "it", ModuleName: "module", ActionType: "view"},
		{Code: "it.module.dashboard.view", Name: "View IT Dashboard", ServiceName: "it", ModuleName: "module", ActionType: "view"},

		// HR — module-level permissions
		{Code: "hr.module.root.view", Name: "Access HR Module", ServiceName: "hr", ModuleName: "module", ActionType: "view"},
		{Code: "hr.module.dashboard.view", Name: "View HR Dashboard", ServiceName: "hr", ModuleName: "module", ActionType: "view"},

		// CI — module-level permissions
		{Code: "ci.module.root.view", Name: "Access CI Module", ServiceName: "ci", ModuleName: "module", ActionType: "view"},
		{Code: "ci.module.dashboard.view", Name: "View CI Dashboard", ServiceName: "ci", ModuleName: "module", ActionType: "view"},

		// Export Import — module-level permissions
		{Code: "exsim.module.root.view", Name: "Access Export Import Module", ServiceName: "exsim", ModuleName: "module", ActionType: "view"},
		{Code: "exsim.module.dashboard.view", Name: "View Export Import Dashboard", ServiceName: "exsim", ModuleName: "module", ActionType: "view"},
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

	// Resolve admin credentials from environment
	appEnv := cfg.App.Env
	creds := getAdminCredentials(appEnv)

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

		// 4. Seed admin user (credentials from env vars)
		adminUserID, err := seedAdminUser(ctx, tx, creds)
		if err != nil {
			return fmt.Errorf("failed to seed admin user: %w", err)
		}

		// 5. Seed user-role assignment
		if err := seedUserRoles(ctx, tx, adminUserID, roleIDs); err != nil {
			return fmt.Errorf("failed to seed user roles: %w", err)
		}

		// 6. Seed menu-permission linkage
		// (Migration 000009 inserts menu rows but can't link permissions because
		//  mst_permission is empty at migration time — we fix that here.)
		if err := seedMenuPermissions(ctx, tx, permIDs); err != nil {
			return fmt.Errorf("failed to seed menu permissions: %w", err)
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
		"iam.rbac.role.delete":       true,
		"iam.rbac.permission.delete": true,
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
		"iam.rbac.role":       true,
		"iam.rbac.permission": true,
	}
	userPerms := make([]string, 0)
	for _, code := range allPermCodes {
		parts := strings.Split(code, ".")
		action := parts[len(parts)-1]

		if action == "view" {
			userPerms = append(userPerms, code)
		} else if len(parts) >= 4 {
			resource := strings.Join(parts[:3], ".")
			if (action == "create" || action == "update" || action == "export" || action == "import") && !adminResources[resource] {
				userPerms = append(userPerms, code)
			}
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
// Credentials are read from environment variables (see getAdminCredentials).
func seedAdminUser(ctx context.Context, tx *sql.Tx, creds adminCredentials) (uuid.UUID, error) {
	log.Info().Msg("Seeding admin user...")

	passwordHash, err := password.Hash(creds.Password)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Check if admin user already exists (partial unique index doesn't support ON CONFLICT)
	var existingID uuid.UUID
	err = tx.QueryRowContext(ctx,
		`SELECT user_id FROM mst_user WHERE username = $1 AND deleted_at IS NULL`, creds.Username,
	).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Admin doesn't exist — insert
		existingID = uuid.New()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO mst_user (user_id, username, email, password_hash, is_active, created_by)
			VALUES ($1, $2, $3, $4, true, $5)`,
			existingID, creds.Username, creds.Email, passwordHash, systemUser,
		)
		if err != nil {
			return uuid.Nil, fmt.Errorf("failed to insert admin user: %w", err)
		}
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("failed to check admin user: %w", err)
	}

	// Seed admin user detail (uq_user_detail_user is absolute UNIQUE, ON CONFLICT works)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO mst_user_detail (detail_id, user_id, employee_code, full_name, first_name, last_name, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO NOTHING`,
		uuid.New(), existingID, "EMP-000", "System Administrator", "System", "Administrator", systemUser,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to insert admin user detail: %w", err)
	}

	log.Info().Str("user_id", existingID.String()).Str("username", creds.Username).Msg("Admin user seeded")
	return existingID, nil
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

// menuPermLink maps a menu_code to the permission codes that gate visibility.
type menuPermLink struct {
	MenuCode  string
	PermCodes []string
}

// defaultMenuPermissions returns the menu → permission linkage.
// A user needs ANY ONE of the linked permissions to see the menu.
// Menus not listed here (e.g., Dashboard) have no requirements → visible to all authenticated users.
func defaultMenuPermissions() []menuPermLink {
	return []menuPermLink{
		// Finance module
		{MenuCode: "FINANCE", PermCodes: []string{"finance.module.root.view"}},
		{MenuCode: "FINANCE_DASHBOARD", PermCodes: []string{"finance.module.dashboard.view"}},
		{MenuCode: "FINANCE_MASTER", PermCodes: []string{"finance.module.master.view"}},
		{MenuCode: "FINANCE_TRANSACTION", PermCodes: []string{"finance.module.transaction.view"}},
		{MenuCode: "FINANCE_UOM", PermCodes: []string{"finance.master.uom.view"}},
		{MenuCode: "FINANCE_PARAMETERS", PermCodes: []string{"finance.master.parameters.view"}},
		{MenuCode: "FINANCE_COSTING", PermCodes: []string{"finance.transaction.costing.view"}},

		// IT module
		{MenuCode: "IT", PermCodes: []string{"it.module.root.view"}},
		{MenuCode: "IT_DASHBOARD", PermCodes: []string{"it.module.dashboard.view"}},

		// HR module
		{MenuCode: "HR", PermCodes: []string{"hr.module.root.view"}},
		{MenuCode: "HR_DASHBOARD", PermCodes: []string{"hr.module.dashboard.view"}},

		// CI module
		{MenuCode: "CI", PermCodes: []string{"ci.module.root.view"}},
		{MenuCode: "CI_DASHBOARD", PermCodes: []string{"ci.module.dashboard.view"}},

		// Export Import module
		{MenuCode: "EXSIM", PermCodes: []string{"exsim.module.root.view"}},
		{MenuCode: "EXSIM_DASHBOARD", PermCodes: []string{"exsim.module.dashboard.view"}},

		// Administrator — requires any one of the IAM management permissions
		{MenuCode: "ADMINISTRATOR", PermCodes: []string{"iam.user.account.view", "iam.rbac.role.view", "iam.rbac.permission.view", "iam.menu.menu.view"}},
		{MenuCode: "ADMIN_USERS", PermCodes: []string{"iam.user.account.view"}},
		{MenuCode: "ADMIN_ROLES", PermCodes: []string{"iam.rbac.role.view"}},
		{MenuCode: "ADMIN_PERMISSIONS", PermCodes: []string{"iam.rbac.permission.view"}},
		{MenuCode: "ADMIN_MENUS", PermCodes: []string{"iam.menu.menu.view"}},
	}
}

// seedMenuPermissions links menus to their required permissions.
func seedMenuPermissions(ctx context.Context, tx *sql.Tx, permIDs map[string]uuid.UUID) error {
	log.Info().Msg("Seeding menu-permission linkage...")

	links := defaultMenuPermissions()
	totalLinked := 0

	for _, link := range links {
		// Look up menu_id by menu_code
		var menuID uuid.UUID
		err := tx.QueryRowContext(ctx,
			`SELECT menu_id FROM mst_menu WHERE menu_code = $1`, link.MenuCode,
		).Scan(&menuID)
		if err != nil {
			// Menu may not exist yet (e.g., migrations not run) — skip gracefully
			log.Warn().Str("menu_code", link.MenuCode).Msg("Menu not found, skipping permission link")
			continue
		}

		for _, permCode := range link.PermCodes {
			permID, ok := permIDs[permCode]
			if !ok {
				log.Warn().Str("permission_code", permCode).Msg("Permission not found, skipping")
				continue
			}

			_, err := tx.ExecContext(ctx, `
				INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
				VALUES ($1, $2, $3)
				ON CONFLICT (menu_id, permission_id) DO NOTHING`,
				menuID, permID, systemUser,
			)
			if err != nil {
				return fmt.Errorf("failed to link permission %s to menu %s: %w", permCode, link.MenuCode, err)
			}
			totalLinked++
		}
	}

	log.Info().Int("total_links", totalLinked).Msg("Menu-permission linkage seeded")
	return nil
}
