// Package grpc provides gRPC server implementation for IAM service.
package grpc

import (
	"context"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// PermissionRequirement defines what's needed to access a method.
type PermissionRequirement struct {
	// Permission is the required permission code (e.g., "iam.user.account.view").
	// Empty string means only authentication is required (no specific permission).
	Permission string
}

// methodPermissions maps gRPC full method names to their required permissions.
// Methods not listed here and not in publicMethods will be denied by default.
var methodPermissions = map[string]PermissionRequirement{
	// Auth Service — authenticated only (no specific permission)
	"/iam.v1.AuthService/GetCurrentUser": {Permission: ""},
	"/iam.v1.AuthService/UpdatePassword": {Permission: ""},
	"/iam.v1.AuthService/Enable2FA":      {Permission: ""},
	"/iam.v1.AuthService/Verify2FA":      {Permission: ""},
	"/iam.v1.AuthService/Disable2FA":     {Permission: ""},

	// User Service
	"/iam.v1.UserService/CreateUser":                 {Permission: "iam.user.account.create"},
	"/iam.v1.UserService/GetUser":                    {Permission: "iam.user.account.view"},
	"/iam.v1.UserService/GetUserDetail":              {Permission: "iam.user.account.view"},
	"/iam.v1.UserService/UpdateUser":                 {Permission: "iam.user.account.update"},
	"/iam.v1.UserService/UpdateUserDetail":           {Permission: "iam.user.account.update"},
	"/iam.v1.UserService/DeleteUser":                 {Permission: "iam.user.account.delete"},
	"/iam.v1.UserService/ListUsers":                  {Permission: "iam.user.account.view"},
	"/iam.v1.UserService/ExportUsers":                {Permission: "iam.user.account.export"},
	"/iam.v1.UserService/ImportUsers":                {Permission: "iam.user.account.import"},
	"/iam.v1.UserService/DownloadTemplate":           {Permission: "iam.user.account.view"},
	"/iam.v1.UserService/AssignUserRoles":            {Permission: "iam.rbac.role.update"},
	"/iam.v1.UserService/RemoveUserRoles":            {Permission: "iam.rbac.role.update"},
	"/iam.v1.UserService/AssignUserPermissions":      {Permission: "iam.rbac.permission.update"},
	"/iam.v1.UserService/RemoveUserPermissions":      {Permission: "iam.rbac.permission.update"},
	"/iam.v1.UserService/GetUserRolesAndPermissions": {Permission: "iam.user.account.view"},

	// Role Service
	"/iam.v1.RoleService/CreateRole":            {Permission: "iam.rbac.role.create"},
	"/iam.v1.RoleService/GetRole":               {Permission: "iam.rbac.role.view"},
	"/iam.v1.RoleService/UpdateRole":            {Permission: "iam.rbac.role.update"},
	"/iam.v1.RoleService/DeleteRole":            {Permission: "iam.rbac.role.delete"},
	"/iam.v1.RoleService/ListRoles":             {Permission: "iam.rbac.role.view"},
	"/iam.v1.RoleService/ExportRoles":           {Permission: "iam.rbac.role.export"},
	"/iam.v1.RoleService/ImportRoles":           {Permission: "iam.rbac.role.import"},
	"/iam.v1.RoleService/DownloadRoleTemplate":  {Permission: "iam.rbac.role.view"},
	"/iam.v1.RoleService/AssignRolePermissions": {Permission: "iam.rbac.role.update"},
	"/iam.v1.RoleService/RemoveRolePermissions": {Permission: "iam.rbac.role.update"},
	"/iam.v1.RoleService/GetRolePermissions":    {Permission: "iam.rbac.role.view"},

	// Permission Service
	"/iam.v1.PermissionService/CreatePermission":           {Permission: "iam.rbac.permission.create"},
	"/iam.v1.PermissionService/GetPermission":              {Permission: "iam.rbac.permission.view"},
	"/iam.v1.PermissionService/UpdatePermission":           {Permission: "iam.rbac.permission.update"},
	"/iam.v1.PermissionService/DeletePermission":           {Permission: "iam.rbac.permission.delete"},
	"/iam.v1.PermissionService/ListPermissions":            {Permission: "iam.rbac.permission.view"},
	"/iam.v1.PermissionService/ExportPermissions":          {Permission: "iam.rbac.permission.export"},
	"/iam.v1.PermissionService/ImportPermissions":          {Permission: "iam.rbac.permission.import"},
	"/iam.v1.PermissionService/DownloadPermissionTemplate": {Permission: "iam.rbac.permission.view"},
	"/iam.v1.PermissionService/GetPermissionsByService":    {Permission: "iam.rbac.permission.view"},

	// Company Service
	"/iam.v1.CompanyService/CreateCompany":           {Permission: "iam.organization.company.create"},
	"/iam.v1.CompanyService/GetCompany":              {Permission: "iam.organization.company.view"},
	"/iam.v1.CompanyService/UpdateCompany":           {Permission: "iam.organization.company.update"},
	"/iam.v1.CompanyService/DeleteCompany":           {Permission: "iam.organization.company.delete"},
	"/iam.v1.CompanyService/ListCompanies":           {Permission: "iam.organization.company.view"},
	"/iam.v1.CompanyService/ExportCompanies":         {Permission: "iam.organization.company.export"},
	"/iam.v1.CompanyService/ImportCompanies":         {Permission: "iam.organization.company.import"},
	"/iam.v1.CompanyService/DownloadCompanyTemplate": {Permission: "iam.organization.company.view"},

	// Division Service
	"/iam.v1.DivisionService/CreateDivision":           {Permission: "iam.organization.division.create"},
	"/iam.v1.DivisionService/GetDivision":              {Permission: "iam.organization.division.view"},
	"/iam.v1.DivisionService/UpdateDivision":           {Permission: "iam.organization.division.update"},
	"/iam.v1.DivisionService/DeleteDivision":           {Permission: "iam.organization.division.delete"},
	"/iam.v1.DivisionService/ListDivisions":            {Permission: "iam.organization.division.view"},
	"/iam.v1.DivisionService/ExportDivisions":          {Permission: "iam.organization.division.export"},
	"/iam.v1.DivisionService/ImportDivisions":          {Permission: "iam.organization.division.import"},
	"/iam.v1.DivisionService/DownloadDivisionTemplate": {Permission: "iam.organization.division.view"},

	// Department Service
	"/iam.v1.DepartmentService/CreateDepartment":           {Permission: "iam.organization.department.create"},
	"/iam.v1.DepartmentService/GetDepartment":              {Permission: "iam.organization.department.view"},
	"/iam.v1.DepartmentService/UpdateDepartment":           {Permission: "iam.organization.department.update"},
	"/iam.v1.DepartmentService/DeleteDepartment":           {Permission: "iam.organization.department.delete"},
	"/iam.v1.DepartmentService/ListDepartments":            {Permission: "iam.organization.department.view"},
	"/iam.v1.DepartmentService/ExportDepartments":          {Permission: "iam.organization.department.export"},
	"/iam.v1.DepartmentService/ImportDepartments":          {Permission: "iam.organization.department.import"},
	"/iam.v1.DepartmentService/DownloadDepartmentTemplate": {Permission: "iam.organization.department.view"},

	// Section Service
	"/iam.v1.SectionService/CreateSection":           {Permission: "iam.organization.section.create"},
	"/iam.v1.SectionService/GetSection":              {Permission: "iam.organization.section.view"},
	"/iam.v1.SectionService/UpdateSection":           {Permission: "iam.organization.section.update"},
	"/iam.v1.SectionService/DeleteSection":           {Permission: "iam.organization.section.delete"},
	"/iam.v1.SectionService/ListSections":            {Permission: "iam.organization.section.view"},
	"/iam.v1.SectionService/ExportSections":          {Permission: "iam.organization.section.export"},
	"/iam.v1.SectionService/ImportSections":          {Permission: "iam.organization.section.import"},
	"/iam.v1.SectionService/DownloadSectionTemplate": {Permission: "iam.organization.section.view"},

	// Organization Service
	"/iam.v1.OrganizationService/GetOrganizationTree": {Permission: "iam.organization.company.view"},

	// Session Service
	"/iam.v1.SessionService/GetCurrentSession":  {Permission: ""},
	"/iam.v1.SessionService/RevokeSession":      {Permission: "iam.session.session.delete"},
	"/iam.v1.SessionService/ListActiveSessions": {Permission: "iam.session.session.view"},

	// Audit Service
	"/iam.v1.AuditService/GetAuditLog":     {Permission: "iam.audit.log.view"},
	"/iam.v1.AuditService/ListAuditLogs":   {Permission: "iam.audit.log.view"},
	"/iam.v1.AuditService/ExportAuditLogs": {Permission: "iam.audit.log.export"},
	"/iam.v1.AuditService/GetAuditSummary": {Permission: "iam.audit.log.view"},

	// Menu Service
	"/iam.v1.MenuService/CreateMenu":            {Permission: "iam.menu.menu.create"},
	"/iam.v1.MenuService/GetMenu":               {Permission: "iam.menu.menu.view"},
	"/iam.v1.MenuService/UpdateMenu":            {Permission: "iam.menu.menu.update"},
	"/iam.v1.MenuService/DeleteMenu":            {Permission: "iam.menu.menu.delete"},
	"/iam.v1.MenuService/ListMenus":             {Permission: "iam.menu.menu.view"},
	"/iam.v1.MenuService/ExportMenus":           {Permission: "iam.menu.menu.export"},
	"/iam.v1.MenuService/ImportMenus":           {Permission: "iam.menu.menu.import"},
	"/iam.v1.MenuService/DownloadMenuTemplate":  {Permission: "iam.menu.menu.view"},
	"/iam.v1.MenuService/GetMenuTree":           {Permission: ""},
	"/iam.v1.MenuService/GetFullMenuTree":       {Permission: "iam.menu.menu.view"},
	"/iam.v1.MenuService/AssignMenuPermissions": {Permission: "iam.menu.menu.update"},
	"/iam.v1.MenuService/RemoveMenuPermissions": {Permission: "iam.menu.menu.update"},
	"/iam.v1.MenuService/GetMenuPermissions":    {Permission: "iam.menu.menu.view"},
	"/iam.v1.MenuService/ReorderMenus":          {Permission: "iam.menu.menu.update"},

	// CMS Page Service
	"/iam.v1.CMSPageService/CreateCMSPage": {Permission: "iam.cms.page.create"},
	"/iam.v1.CMSPageService/GetCMSPage":    {Permission: "iam.cms.page.view"},
	"/iam.v1.CMSPageService/UpdateCMSPage": {Permission: "iam.cms.page.update"},
	"/iam.v1.CMSPageService/DeleteCMSPage": {Permission: "iam.cms.page.delete"},
	"/iam.v1.CMSPageService/ListCMSPages":  {Permission: "iam.cms.page.view"},

	// CMS Section Service
	"/iam.v1.CMSSectionService/CreateCMSSection": {Permission: "iam.cms.section.create"},
	"/iam.v1.CMSSectionService/GetCMSSection":    {Permission: "iam.cms.section.view"},
	"/iam.v1.CMSSectionService/UpdateCMSSection": {Permission: "iam.cms.section.update"},
	"/iam.v1.CMSSectionService/DeleteCMSSection": {Permission: "iam.cms.section.delete"},
	"/iam.v1.CMSSectionService/ListCMSSections":  {Permission: "iam.cms.section.view"},
	"/iam.v1.CMSSectionService/UploadCMSImage":   {Permission: "iam.cms.section.create"},

	// CMS Setting Service
	"/iam.v1.CMSSettingService/GetCMSSetting":         {Permission: "iam.cms.setting.view"},
	"/iam.v1.CMSSettingService/UpdateCMSSetting":      {Permission: "iam.cms.setting.update"},
	"/iam.v1.CMSSettingService/ListCMSSettings":       {Permission: "iam.cms.setting.view"},
	"/iam.v1.CMSSettingService/BulkUpdateCMSSettings": {Permission: "iam.cms.setting.update"},
}

// PermissionInterceptor creates a unary interceptor that checks if the
// authenticated user has the required permission for the requested method.
func PermissionInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Skip for public endpoints (already handled by AuthInterceptor)
		if isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		// Super admin bypasses all permission checks
		if IsSuperAdmin(ctx) {
			return handler(ctx, req)
		}

		// Look up required permission for this method
		requirement, exists := methodPermissions[info.FullMethod]
		if !exists {
			// Deny by default for unmapped methods — this is a security measure.
			// If a new RPC is added but not registered here, it will be blocked
			// until explicitly configured.
			log.Warn().
				Str("method", info.FullMethod).
				Msg("Permission check: unmapped method denied")
			return nil, status.Error(codes.PermissionDenied, "access denied")
		}

		// Empty permission means authenticated-only (no specific permission required)
		if requirement.Permission == "" {
			return handler(ctx, req)
		}

		// Check if user has the required permission
		if !HasPermission(ctx, requirement.Permission) {
			log.Debug().
				Str("method", info.FullMethod).
				Str("required", requirement.Permission).
				Strs("user_permissions", GetPermissionsFromCtx(ctx)).
				Msg("Permission denied")
			return nil, status.Error(codes.PermissionDenied, "insufficient permissions")
		}

		return handler(ctx, req)
	}
}
