// Package grpc provides gRPC server implementation.
package grpc

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// Auth context keys.
const (
	AuthUserIDKey      ContextKey = "auth_user_id"
	AuthUsernameKey    ContextKey = "auth_username"
	AuthEmailKey       ContextKey = "auth_email"
	AuthRolesKey       ContextKey = "auth_roles"
	AuthPermissionsKey ContextKey = "auth_permissions"
)

// JWTClaims mirrors the IAM service JWT claims structure.
type JWTClaims struct {
	jwt.RegisteredClaims
	TokenType   string   `json:"token_type"`
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// TokenBlacklistChecker checks if a token has been revoked.
type TokenBlacklistChecker interface {
	IsBlacklisted(ctx context.Context, tokenID string) (bool, error)
}

// PermissionsReader fetches a user's permissions from the IAM Redis cache.
type PermissionsReader interface {
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
}

// internalLookupMethods are the read-only CostMasterLookupService RPCs invoked
// service-to-service by PPC (financeclient). They carry no user JWT; they are
// authenticated by the shared internal secret and are safe to expose to trusted
// callers because they never mutate finance state.
var internalLookupMethods = map[string]struct{}{
	"/finance.v1.CostMasterLookupService/GetCostProductMasterForPPC":     {},
	"/finance.v1.CostMasterLookupService/BatchGetCostProductMaster":      {},
	"/finance.v1.CostMasterLookupService/ListCostProductMasterForPPC":    {},
	"/finance.v1.CostMasterLookupService/GetProductRouteForPPC":          {},
	"/finance.v1.CostMasterLookupService/ListProductGradesForPPC":        {},
	"/finance.v1.CostMasterLookupService/ListProductParametersForPPC":    {},
	"/finance.v1.CostMasterLookupService/BatchGetProductParameterValues": {},
	// ResolveCostProductMasterByErpCode links PPC sales-order staging rows to
	// finance products by ERP item/shade code. Read-only, same trust boundary.
	"/finance.v1.CostMasterLookupService/ResolveCostProductMasterByErpCode": {},
}

// dualAuthMethods are RPCs called BOTH by trusted internal services (via the
// shared service secret, no user JWT) AND by end users through the normal
// Finance UI (via a user JWT). Unlike internalLookupMethods, a missing service
// secret header does NOT reject the request — it falls through to standard
// JWT authentication instead.
var dualAuthMethods = map[string]struct{}{
	// MachineService.ListMachines is read both by PPC's machine-sync worker
	// (finance mst_machine → PPC machine, merged with Oracle TXTMACH) via the
	// internal service secret, and by the Finance UI's Machine master page via
	// a normal user JWT. It must not require the internal secret for the UI.
	"/finance.v1.MachineService/ListMachines": {},
}

// hasServiceSecretHeader reports whether the request carries either of the
// internal service-secret headers, regardless of whether the value matches.
func hasServiceSecretHeader(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, header := range []string{"x-service-secret", "x-internal-token"} {
		if len(md.Get(header)) > 0 {
			return true
		}
	}
	return false
}

// serviceSecretValid reports whether the request carries a matching internal
// service secret. The secret may arrive in either the x-service-secret header
// (finance-cost-worker precedent) or the x-internal-token header (PPC
// financeclient — the header name it already sends). An empty configured secret
// skips the check (trusts cluster network isolation).
func serviceSecretValid(ctx context.Context, svcSecret string) bool {
	if svcSecret == "" {
		return true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, header := range []string{"x-service-secret", "x-internal-token"} {
		vals := md.Get(header)
		if len(vals) == 1 && vals[0] == svcSecret {
			return true
		}
	}
	return false
}

// withServiceIdentity injects a synthetic SUPER_ADMIN identity for trusted
// service-to-service calls so the permission interceptor's bypass applies.
func withServiceIdentity(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, AuthUserIDKey, "service:finance-cost-worker")
	ctx = context.WithValue(ctx, AuthUsernameKey, "finance-cost-worker")
	ctx = context.WithValue(ctx, AuthRolesKey, []string{"SUPER_ADMIN"})
	ctx = context.WithValue(ctx, AuthPermissionsKey, []string{})
	return ctx
}

// AuthInterceptor validates JWT tokens issued by IAM service.
// blacklist is optional — if nil, blacklist checking is skipped (graceful degradation).
// permsReader is optional — if nil, permissions fall back to JWT claims (empty after cookie-size fix).
func AuthInterceptor(cfg *config.JWTConfig, blacklist TokenBlacklistChecker, permsReader PermissionsReader) grpc.UnaryServerInterceptor { //nolint:gocognit,gocyclo // sequential auth gates, cohesive
	secret := []byte(cfg.AccessTokenSecret)
	svcSecret := cfg.ServiceSecret

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Health checks are always public.
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") ||
			strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		// ProcessChunkInternal is invoked by finance-cost-worker via the cluster's
		// internal network. The RPC has NO HTTP gateway path (proto file omits
		// google.api.http annotation by design), so it's not reachable from the
		// public internet. We trust network isolation for service-to-service
		// auth + inject synthetic SUPER_ADMIN identity so the permission
		// interceptor's SUPER_ADMIN bypass takes effect.
		//
		// When cfg.ServiceSecret is set, also require x-service-secret header
		// match for defense-in-depth.
		if info.FullMethod == "/finance.v1.CostCalcService/ProcessChunkInternal" {
			if !serviceSecretValid(ctx, svcSecret) {
				return nil, status.Error(codes.Unauthenticated, "ProcessChunkInternal: missing or invalid x-service-secret")
			}
			return handler(withServiceIdentity(ctx), req)
		}

		// CostMasterLookupService RPCs are read-only master projections consumed
		// by the PPC service over the internal network. They carry no user JWT;
		// a valid internal service secret (x-service-secret OR x-internal-token)
		// grants a synthetic SUPER_ADMIN identity so the permission interceptor's
		// bypass applies. This does NOT relax auth for any user-facing RPC.
		if _, ok := internalLookupMethods[info.FullMethod]; ok {
			if !serviceSecretValid(ctx, svcSecret) {
				return nil, status.Error(codes.Unauthenticated, "lookup: missing or invalid internal service secret")
			}
			return handler(withServiceIdentity(ctx), req)
		}

		// dualAuthMethods: only take the internal-secret shortcut when the
		// caller actually presented a service-secret header (and it's valid).
		// Otherwise fall through to normal user JWT auth below, so the
		// Finance UI's own JWT-authenticated calls keep working.
		if _, ok := dualAuthMethods[info.FullMethod]; ok && hasServiceSecretHeader(ctx) {
			if !serviceSecretValid(ctx, svcSecret) {
				return nil, status.Error(codes.Unauthenticated, "lookup: missing or invalid internal service secret")
			}
			return handler(withServiceIdentity(ctx), req)
		}

		// All finance endpoints require authentication.
		token, err := extractBearerToken(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "missing or invalid authorization: %v", err)
		}

		claims, err := validateAccessToken(token, secret)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Check token blacklist (cross-service logout enforcement).
		if blacklist != nil && claims.ID != "" {
			blacklisted, blErr := blacklist.IsBlacklisted(ctx, claims.ID)
			if blErr != nil {
				log.Warn().Err(blErr).Msg("Failed to check token blacklist")
				// Fail-open: continue if blacklist check fails.
				// Short access token TTL (15min) limits exposure.
			}
			if blacklisted {
				return nil, status.Error(codes.Unauthenticated, "token has been revoked")
			}
		}

		// Resolve permissions from IAM Redis cache (JWT no longer embeds them).
		// Fall back to claims.Permissions on cache miss so old tokens still work.
		perms := claims.Permissions
		if permsReader != nil {
			if cached, err := permsReader.GetUserPermissions(ctx, claims.UserID); err != nil {
				log.Warn().Err(err).Str("userID", claims.UserID).Msg("failed to fetch permissions from Redis, using JWT fallback")
			} else if cached != nil {
				perms = cached
			}
		}

		// Populate context with user info from claims.
		ctx = context.WithValue(ctx, AuthUserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, AuthUsernameKey, claims.Username)
		ctx = context.WithValue(ctx, AuthEmailKey, claims.Email)
		ctx = context.WithValue(ctx, AuthRolesKey, claims.Roles)
		ctx = context.WithValue(ctx, AuthPermissionsKey, perms)

		return handler(ctx, req)
	}
}

func extractBearerToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("no metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return "", errors.New("no authorization header")
	}

	authHeader := values[0]
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", errors.New("invalid authorization format")
	}

	return strings.TrimPrefix(authHeader, "Bearer "), nil
}

func validateAccessToken(tokenString string, secret []byte) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("token expired")
		}
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	if claims.TokenType != "access" {
		return nil, errors.New("not an access token")
	}

	return claims, nil
}

// GetUserIDFromCtx retrieves the user ID from context.
func GetUserIDFromCtx(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AuthUserIDKey).(string)
	return val, ok
}

// GetUsernameFromCtx retrieves the username from context.
func GetUsernameFromCtx(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(AuthUsernameKey).(string)
	return val, ok
}

// GetRolesFromCtx retrieves the roles from context.
func GetRolesFromCtx(ctx context.Context) []string {
	roles, ok := ctx.Value(AuthRolesKey).([]string)
	if !ok {
		return nil
	}
	return roles
}

// GetPermissionsFromCtx retrieves the permissions from context.
func GetPermissionsFromCtx(ctx context.Context) []string {
	perms, ok := ctx.Value(AuthPermissionsKey).([]string)
	if !ok {
		return nil
	}
	return perms
}

// HasPermission checks if the user has a specific permission.
func HasPermission(ctx context.Context, permission string) bool {
	return slices.Contains(GetPermissionsFromCtx(ctx), permission)
}

// HasRole checks if the user has a specific role.
func HasRole(ctx context.Context, role string) bool {
	return slices.Contains(GetRolesFromCtx(ctx), role)
}

// IsSuperAdmin checks if the user has the SUPER_ADMIN role.
func IsSuperAdmin(ctx context.Context) bool {
	return HasRole(ctx, "SUPER_ADMIN")
}

// PermissionInterceptor enforces RBAC permission checks for Finance service methods.
func PermissionInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// Skip for health/reflection.
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") ||
			strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		// SUPER_ADMIN bypasses all permission checks.
		if IsSuperAdmin(ctx) {
			return handler(ctx, req)
		}

		required := getRequiredPermission(info.FullMethod)
		if required == "" {
			// No specific permission needed — authenticated access is sufficient.
			return handler(ctx, req)
		}

		if !HasPermission(ctx, required) {
			log.Warn().
				Str("method", info.FullMethod).
				Str("required", required).
				Msg("Permission denied")
			return nil, status.Errorf(codes.PermissionDenied, "permission denied: requires %s", required)
		}

		return handler(ctx, req)
	}
}

// getRequiredPermission returns the permission code needed for a method.
func getRequiredPermission(fullMethod string) string {
	// Permission mapping for Finance service.
	// Format: {service}.{module}.{entity}.{action}
	permissions := map[string]string{
		// UOM Service
		"/finance.v1.UOMService/CreateUOM":  "finance.master.uom.create",
		"/finance.v1.UOMService/GetUOM":     "finance.master.uom.view",
		"/finance.v1.UOMService/ListUOMs":   "finance.master.uom.view",
		"/finance.v1.UOMService/UpdateUOM":  "finance.master.uom.update",
		"/finance.v1.UOMService/DeleteUOM":  "finance.master.uom.delete",
		"/finance.v1.UOMService/ImportUOMs": "finance.master.uom.create",
		"/finance.v1.UOMService/ExportUOMs": "finance.master.uom.view",

		// RM Category Service
		"/finance.v1.RMCategoryService/CreateRMCategory":           "finance.master.rmcategory.create",
		"/finance.v1.RMCategoryService/GetRMCategory":              "finance.master.rmcategory.view",
		"/finance.v1.RMCategoryService/ListRMCategories":           "finance.master.rmcategory.view",
		"/finance.v1.RMCategoryService/UpdateRMCategory":           "finance.master.rmcategory.update",
		"/finance.v1.RMCategoryService/DeleteRMCategory":           "finance.master.rmcategory.delete",
		"/finance.v1.RMCategoryService/ImportRMCategories":         "finance.master.rmcategory.import",
		"/finance.v1.RMCategoryService/ExportRMCategories":         "finance.master.rmcategory.export",
		"/finance.v1.RMCategoryService/DownloadRMCategoryTemplate": "finance.master.rmcategory.view",

		// CostCalc Service (S8a foundation; stubs return Unimplemented).
		"/finance.v1.CostCalcService/TriggerCalcJob":      "finance.cost.caljob.trigger",
		"/finance.v1.CostCalcService/GetCalcJob":          "finance.cost.caljob.view",
		"/finance.v1.CostCalcService/ListCalcJobs":        "finance.cost.caljob.view",
		"/finance.v1.CostCalcService/ListCalcJobChunks":   "finance.cost.caljob.view",
		"/finance.v1.CostCalcService/ListCalcJobProducts": "finance.cost.caljob.view",
		"/finance.v1.CostCalcService/CancelCalcJob":       "finance.cost.caljob.cancel",
		"/finance.v1.CostCalcService/GetCostResult":       "finance.cost.result.view",
		"/finance.v1.CostCalcService/GetCostBreakdown":    "finance.cost.result.view",
		"/finance.v1.CostCalcService/ListCostHistory":     "finance.cost.history.view",
		"/finance.v1.CostCalcService/VerifyCostResult":    "finance.cost.result.verify",
		"/finance.v1.CostCalcService/ApproveCostResult":   "finance.cost.result.approve",
		// Service-to-service: invoked by finance-cost-worker. Same scope as triggering a job.
		"/finance.v1.CostCalcService/ProcessChunkInternal": "finance.cost.caljob.trigger",

		// MBHeadService
		"/finance.v1.MBHeadService/CreateMBHead":  "finance.mb.head.create",
		"/finance.v1.MBHeadService/GetMBHead":     "finance.mb.head.view",
		"/finance.v1.MBHeadService/ListMBHeads":   "finance.mb.head.view",
		"/finance.v1.MBHeadService/UpdateMBHead":  "finance.mb.head.update",
		"/finance.v1.MBHeadService/DeleteMBHead":  "finance.mb.head.delete",
		"/finance.v1.MBHeadService/ExportMBHeads": "finance.mb.head.view",
		// P12: the denormalized full-recipe export gates on the dedicated recipe-export
		// permission seeded by iam migration 000083, NOT on finance.mb.head.view — it
		// discloses composition and MB cost, which plain head viewing does not.
		"/finance.v1.MBHeadService/ExportMBRecipeFull":     "finance.mb.recipe.export",
		"/finance.v1.MBHeadService/ImportMBHeads":          "finance.mb.head.create",
		"/finance.v1.MBHeadService/DownloadMBHeadTemplate": "finance.mb.head.view",
		"/finance.v1.MBHeadService/SubmitMBHead":           "finance.mb.head.submit",
		"/finance.v1.MBHeadService/ApproveMBHead":          "finance.mb.head.approve",
		"/finance.v1.MBHeadService/ValidateMBHead":         "finance.mb.head.validate",
		// 🔴 USER DECISION 2026-08-26 — Un-approve and Revoke were REMOVED from the MB
		// Recipe workflow. Both RPCs now refuse every call with a 410 BaseResponse
		// (mb_head_handler.go), and their application handlers refuse too.
		//
		// ⛔ These two mappings are deliberately KEPT anyway. getRequiredPermission
		// returns "" for any RPC missing from this map, and PermissionInterceptor reads
		// "" as "any authenticated user may call this" — dropping the rows would turn two
		// permission-gated RPCs into open ones and would widen the hole the K-34 (c)
		// coverage ratchet in permission_coverage_test.go exists to freeze. Keeping them
		// costs nothing: the call is refused a layer later regardless.
		"/finance.v1.MBHeadService/UnApproveMBHead": "finance.mb.head.unapprove",
		"/finance.v1.MBHeadService/RevokeMBHead":    "finance.mb.head.revoke",
		"/finance.v1.MBHeadService/RejectMBHead":    "finance.mb.head.reject",
		// K-30 option A: the REJECTED → DRAFT return deliberately reuses the EXISTING
		// submit permission instead of a dedicated one — the author entitled to submit an
		// MB may pull their own MB back to DRAFT to fix it. Zero IAM migrations, zero new
		// seeds. This is not a copy-paste slip.
		"/finance.v1.MBHeadService/ReturnMBHeadToDraft": "finance.mb.head.submit",
		// 2026-08-31: Unrevoke (REVOKED → DRAFT) is gated by a NEW, DEDICATED permission
		// — unlike ReturnMBHeadToDraft above, it does NOT reuse an existing code. Granted
		// only to SUPER_ADMIN via migration 000091; a plain author with submit rights
		// cannot self-service their way out of REVOKED.
		"/finance.v1.MBHeadService/UnrevokeMBHead": "finance.mb.head.unrevoke",
		// P10 lock/unlock — the two sides of the workflow gate on DIFFERENT codes, per
		// an explicit USER DECISION: asking for an unlock and deciding on one are
		// separate entitlements, so a plain requester cannot approve anything.
		//
		//   finance.mb.recipe.unlockrequest → RequestUnlockMBHead  (may ASK)
		//   finance.mb.recipe.unlock        → GrantUnlockMBHead,
		//                                     RejectUnlockMBHead   (may DECIDE)
		//
		// The code is CONCATENATED ("unlockrequest", not "unlock.request") because
		// chk_permission_code_format (iam 000004:55) and its Go mirror
		// (iam internal/domain/role/entity.go:27) accept EXACTLY 4 dot-separated
		// segments. ⛔ Do not "tidy" it into a fifth segment — the seed INSERT would
		// be rejected by the CHECK constraint and this RPC would deny everyone.
		//
		// ⛔ There is deliberately NO self-approval / "is this my own request" identity
		// check anywhere in this flow. The permission split alone produces the behavior
		// the user asked for: a requester without the deciding code is rejected by this
		// map, and someone who holds the deciding code may approve a request they filed
		// themselves — that is intended, ⛔ do not "fix" it with an actor comparison.
		//
		// ⚠ FAIL-CLOSED WARNING: an RPC mapped to a code that no role holds rejects
		// EVERYONE. finance.mb.recipe.unlock is ALREADY SEEDED by iam migration 000083
		// (:27) and granted to MB_APPROVER (:107, :160). The new
		// finance.mb.recipe.unlockrequest code is seeded and granted to all four MB
		// tiers by iam migration 000087 — ⚠ that migration MUST BE APPLIED, or every
		// RequestUnlockMBHead call is denied for EVERY user, not just some.
		"/finance.v1.MBHeadService/RequestUnlockMBHead": "finance.mb.recipe.unlockrequest",
		"/finance.v1.MBHeadService/GrantUnlockMBHead":   "finance.mb.recipe.unlock",
		"/finance.v1.MBHeadService/RejectUnlockMBHead":  "finance.mb.recipe.unlock",

		// MBSpinService (reuses finance.yarnmaster.mbspin.* seeded in iam mig 000057)
		"/finance.v1.MBSpinService/CreateMBSpin":  "finance.yarnmaster.mbspin.create",
		"/finance.v1.MBSpinService/GetMBSpin":     "finance.yarnmaster.mbspin.view",
		"/finance.v1.MBSpinService/ListMBSpins":   "finance.yarnmaster.mbspin.view",
		"/finance.v1.MBSpinService/UpdateMBSpin":  "finance.yarnmaster.mbspin.update",
		"/finance.v1.MBSpinService/DeleteMBSpin":  "finance.yarnmaster.mbspin.delete",
		"/finance.v1.MBSpinService/ExportMBSpins": "finance.yarnmaster.mbspin.view",
		"/finance.v1.MBSpinService/ImportMBSpins": "finance.yarnmaster.mbspin.create",
		// DuplicateMBSpin = tulis (bikin spin baru) ⇒ .create, sama seperti CreateMBSpin/ImportMBSpins.
		// Kode ini sudah di-seed (iam 000057:47) dan sudah dipakai CreateMBSpin ⇒ nol risiko fail-closed.
		"/finance.v1.MBSpinService/DuplicateMBSpin":        "finance.yarnmaster.mbspin.create",
		"/finance.v1.MBSpinService/DownloadMBSpinTemplate": "finance.yarnmaster.mbspin.view",

		// MbCompositionService — composition rows are part of the MB head recipe, reuse its permissions.
		"/finance.v1.MbCompositionService/CreateMbComposition":       "finance.mb.head.update",
		"/finance.v1.MbCompositionService/UpdateMbComposition":       "finance.mb.head.update",
		"/finance.v1.MbCompositionService/DeleteMbComposition":       "finance.mb.head.update",
		"/finance.v1.MbCompositionService/ListMbCompositions":        "finance.mb.head.view",
		"/finance.v1.MbCompositionService/ListMbCompositionVersions": "finance.mb.head.view",

		// MbLustureService (master data)
		"/finance.v1.MbLustureService/CreateMbLusture": "finance.master.mblusture.create",
		"/finance.v1.MbLustureService/UpdateMbLusture": "finance.master.mblusture.update",
		"/finance.v1.MbLustureService/DeleteMbLusture": "finance.master.mblusture.delete",
		"/finance.v1.MbLustureService/GetMbLusture":    "finance.master.mblusture.view",
		"/finance.v1.MbLustureService/ListMbLusture":   "finance.master.mblusture.view",

		// MbCrossSectionService (master data)
		"/finance.v1.MbCrossSectionService/CreateMbCrossSection": "finance.master.mbxsection.create",
		"/finance.v1.MbCrossSectionService/UpdateMbCrossSection": "finance.master.mbxsection.update",
		"/finance.v1.MbCrossSectionService/DeleteMbCrossSection": "finance.master.mbxsection.delete",
		"/finance.v1.MbCrossSectionService/GetMbCrossSection":    "finance.master.mbxsection.view",
		"/finance.v1.MbCrossSectionService/ListMbCrossSection":   "finance.master.mbxsection.view",

		// MbCrossSectionFactorService (master data) — the directed-pair conversion factors.
		//
		// These 5 RPCs previously REUSED the finance.master.mbxsection.* master
		// permissions. User decision K-21 (gate G17-FAKTOR-PERM, 2026-08-22)
		// REJECTED that piggy-backing: conversion factors feed the LDR
		// calculation, so the right to change them must be separable from the
		// right to change the master code list. They now require their own
		// permissions, seeded by IAM migration
		// 000086_seed_mb_cross_section_factor_permissions.up.sql.
		//
		// ⚠ These entries and that migration must ship together: an unmapped RPC
		// FAILS OPEN here (see the `required == ""` branch below), and a mapped
		// RPC whose permission row does not exist fails CLOSED for everyone.
		"/finance.v1.MbCrossSectionFactorService/CreateMbCrossSectionFactor": "finance.master.mbxsectionfactor.create",
		"/finance.v1.MbCrossSectionFactorService/UpdateMbCrossSectionFactor": "finance.master.mbxsectionfactor.update",
		"/finance.v1.MbCrossSectionFactorService/DeleteMbCrossSectionFactor": "finance.master.mbxsectionfactor.delete",
		"/finance.v1.MbCrossSectionFactorService/GetMbCrossSectionFactor":    "finance.master.mbxsectionfactor.view",
		"/finance.v1.MbCrossSectionFactorService/ListMbCrossSectionFactor":   "finance.master.mbxsectionfactor.view",

		// MBDozingService (read-only LDR calculation + impact preview).
		"/finance.v1.MBDozingService/CalculateDozing":     "finance.mb.dozing.calculate",
		"/finance.v1.MBDozingService/PreviewDozingImpact": "finance.mb.dozing.preview",

		// MbParamService (master data)
		"/finance.v1.MbParamService/CreateMbParam":       "finance.master.mbparam.create",
		"/finance.v1.MbParamService/UpdateMbParam":       "finance.master.mbparam.update",
		"/finance.v1.MbParamService/DeleteMbParam":       "finance.master.mbparam.delete",
		"/finance.v1.MbParamService/ListMbParams":        "finance.master.mbparam.view",
		"/finance.v1.MbParamService/CreateMbParamOption": "finance.master.mbparam.create",
		"/finance.v1.MbParamService/UpdateMbParamOption": "finance.master.mbparam.update",
		"/finance.v1.MbParamService/DeleteMbParamOption": "finance.master.mbparam.delete",

		// MbPushService (reuses finance.mb.pushtohead.* seeded in iam mig 000068)
		"/finance.v1.MbPushService/PreviewPushToHead": "finance.mb.pushtohead.preview",
		"/finance.v1.MbPushService/ExecutePushToHead": "finance.mb.pushtohead.execute",
		"/finance.v1.MbPushService/ListMbPushLogs":    "finance.mb.pushtohead.preview",

		// MbWorkflowLogService — read-only, reuse MB Head view permission (no dedicated log-view code seeded).
		"/finance.v1.MbWorkflowLogService/ListMbWorkflowLogs": "finance.mb.head.view",

		// MbBatchService
		"/finance.v1.MbBatchService/TriggerMbBatch": "finance.mb.batch.trigger",

		// CostProductRequestService
		"/finance.v1.CostProductRequestService/CreateCostProductRequest":               "finance.product.request.create",
		"/finance.v1.CostProductRequestService/UpdateCostProductRequest":               "finance.product.request.create",
		"/finance.v1.CostProductRequestService/GetCostProductRequest":                  "finance.product.request.view",
		"/finance.v1.CostProductRequestService/GetCostProductRequestByNo":              "finance.product.request.view",
		"/finance.v1.CostProductRequestService/ListCostProductRequests":                "finance.product.request.view",
		"/finance.v1.CostProductRequestService/SubmitCostProductRequest":               "finance.product.request.submit",
		"/finance.v1.CostProductRequestService/CancelCostProductRequest":               "",
		"/finance.v1.CostProductRequestService/CloseCostProductRequest":                "",
		"/finance.v1.CostProductRequestService/ReviseCostProductRequest":               "finance.product.request.create",
		"/finance.v1.CostProductRequestService/StartCostProductRequestReview":          "finance.product.request.review",
		"/finance.v1.CostProductRequestService/VerifyCostProductRequestClassification": "finance.product.request.review",
		"/finance.v1.CostProductRequestService/DecideCostProductRequestFeasibility":    "finance.product.request.resolve",
		// SubmitAndDecideCostProductRequest merges Submit+StartReview+VerifyClassification+
		// DecideFeasibility+LinkRoute (design.md §3 B3) and is gated SOLELY by the
		// review permission — not also by .submit or .resolve — per the user-approved
		// permission narrowing (see P3-T7's rollout migration for the access impact).
		"/finance.v1.CostProductRequestService/SubmitAndDecideCostProductRequest":       "finance.product.request.review",
		"/finance.v1.CostProductRequestService/UseExistingCostingForCostProductRequest": "finance.product.request.resolve",
		"/finance.v1.CostProductRequestService/RejectCostProductRequest":                "finance.product.request.reject",
		"/finance.v1.CostProductRequestService/AssignCostProductRequest":                "finance.product.request.assign",
		"/finance.v1.CostProductRequestService/MarkParameterComplete":                   "finance.product.request.resolve",
		"/finance.v1.CostProductRequestService/ConfirmCostProductRequest":               "finance.product.request.confirm",
		"/finance.v1.CostProductRequestService/ApproveCostProductRequest":               "finance.product.request.approve",
		"/finance.v1.CostProductRequestService/ReleaseCostProductRequest":               "finance.product.request.release",
		"/finance.v1.CostProductRequestService/ReopenCostProductRequest":                "finance.product.request.reopen",
		"/finance.v1.CostProductRequestService/GetCostProductRequestHistory":            "finance.product.request.view",
		"/finance.v1.CostProductRequestService/LinkExistingRoute":                       "finance.product.route.update",
		"/finance.v1.CostProductRequestService/UnlinkRoute":                             "finance.product.route.update",
		// D6 import/export (design.md §4 Area D6) — mirrors UOM's Export=view/Import=create pattern.
		"/finance.v1.CostProductRequestService/ExportCostProductRequests":           "finance.product.request.view",
		"/finance.v1.CostProductRequestService/ImportCostProductRequests":           "finance.product.request.create",
		"/finance.v1.CostProductRequestService/GetCostProductRequestImportTemplate": "finance.product.request.view",

		// CostRouteService
		"/finance.v1.CostRouteService/CreateRouteFromProduct": "finance.product.route.create",
		"/finance.v1.CostRouteService/GetRouteByProduct":      "finance.product.route.view",
		"/finance.v1.CostRouteService/GetRouteGraph":          "finance.product.route.view",
		"/finance.v1.CostRouteService/SaveRouteGraph":         "finance.product.route.create",
		"/finance.v1.CostRouteService/CompleteRoute":          "finance.product.route.create",
		"/finance.v1.CostRouteService/LockRoute":              "finance.product.route.update",
		"/finance.v1.CostRouteService/UnlockRoute":            "finance.product.route.update",
		"/finance.v1.CostRouteService/DeleteRoute":            "finance.product.route.create",
		"/finance.v1.CostRouteService/ListRoutes":             "finance.product.route.view",
		"/finance.v1.CostRouteService/DuplicateRoute":         "finance.product.route.create",
		"/finance.v1.CostRouteService/ListLinkedRequests":     "finance.product.route.view",

		// CostProductMasterService
		"/finance.v1.CostProductMasterService/CreateCostProductMaster":           "finance.product.route.create",
		"/finance.v1.CostProductMasterService/UpdateCostProductMaster":           "finance.product.route.create",
		"/finance.v1.CostProductMasterService/GetCostProductMaster":              "finance.product.route.view",
		"/finance.v1.CostProductMasterService/GetCostProductMasterByCode":        "finance.product.route.view",
		"/finance.v1.CostProductMasterService/ListCostProductMasters":            "finance.product.route.view",
		"/finance.v1.CostProductMasterService/UpdateCostProductMasterErpLinkage": "finance.product.route.create",
		"/finance.v1.CostProductMasterService/DeactivateCostProductMaster":       "finance.product.route.update",
		"/finance.v1.CostProductMasterService/UnlockCostProductMaster":           "finance.product.route.update",
		// K-43: tiga bulk RPC masuk lingkup "Master Product MB"; Export/DownloadTemplate = baca (.view),
		// Import = tulis massal (.create), mengikuti preseden Create/Update di atas.
		"/finance.v1.CostProductMasterService/ExportCostProductMasters":          "finance.product.route.view",
		"/finance.v1.CostProductMasterService/ImportCostProductMasters":          "finance.product.route.create",
		"/finance.v1.CostProductMasterService/DownloadCostProductMasterTemplate": "finance.product.route.view",

		// CostFillTaskService — authenticated-only (access controlled by fill config domain)
		"/finance.v1.CostFillTaskService/ListFillTasks":   "",
		"/finance.v1.CostFillTaskService/ClaimFillTask":   "",
		"/finance.v1.CostFillTaskService/SubmitFillTask":  "",
		"/finance.v1.CostFillTaskService/ApproveFillTask": "",
		"/finance.v1.CostFillTaskService/RejectFillTask":  "",

		// CostLevelAssignmentConfigService
		"/finance.v1.CostLevelAssignmentConfigService/UpsertLevelConfig":  "finance.product.request.resolve",
		"/finance.v1.CostLevelAssignmentConfigService/DeleteGlobalConfig": "finance.product.request.resolve",
		"/finance.v1.CostLevelAssignmentConfigService/ListGlobalConfigs":  "finance.product.request.view",

		// ShadeService (master shade CRUD, R8) — permissions seeded by iam
		// migration 000089. DeactivateShade maps to .delete: the seed names that
		// code "Deactivate Shade" even though the RPC only soft-deactivates
		// (ces_is_active = false), matching the sibling masters' action naming.
		//
		// ⚠ 000089 assigns all five finance.master.shade.* permissions ONLY to the
		// SUPER_ADMIN role (000089:102-110). SUPER_ADMIN already bypasses this
		// entire permission map via IsSuperAdmin(), so that grant has no practical
		// effect. No other role currently holds any finance.master.shade.*
		// permission — see the auth_interceptor.go task report for the open
		// decision this raises.
		"/finance.v1.ShadeService/CreateShade":     "finance.master.shade.create",
		"/finance.v1.ShadeService/GetShade":        "finance.master.shade.view",
		"/finance.v1.ShadeService/UpdateShade":     "finance.master.shade.update",
		"/finance.v1.ShadeService/DeactivateShade": "finance.master.shade.delete",
		"/finance.v1.ShadeService/ListShades":      "finance.master.shade.view",
		"/finance.v1.ShadeService/SyncShades":      "finance.master.shade.sync",

		// UOMCategoryService — mutating RPCs guarded with the pre-seeded
		// finance.master.uomcategory.* codes (K-34 fail-open closure).
		"/finance.v1.UOMCategoryService/UpdateUOMCategory": "finance.master.uomcategory.update",
		"/finance.v1.UOMCategoryService/DeleteUOMCategory": "finance.master.uomcategory.delete",

		// ParameterService — mutating RPCs guarded with the pre-seeded
		// finance.master.parameter.* codes (K-34 fail-open closure).
		"/finance.v1.ParameterService/UpdateParameter": "finance.master.parameter.update",
		"/finance.v1.ParameterService/DeleteParameter": "finance.master.parameter.delete",

		// FormulaService — mutating RPCs guarded with the pre-seeded
		// finance.master.formula.* codes (K-34 fail-open closure).
		"/finance.v1.FormulaService/UpdateFormula": "finance.master.formula.update",
		"/finance.v1.FormulaService/DeleteFormula": "finance.master.formula.delete",

		// MachineService — mutating RPCs guarded with the pre-seeded
		// finance.yarnmaster.machine.* codes (K-34 fail-open closure).
		"/finance.v1.MachineService/UpdateMachine": "finance.yarnmaster.machine.update",
		"/finance.v1.MachineService/DeleteMachine": "finance.yarnmaster.machine.delete",

		// LookupMasterService — mutating RPCs guarded with the pre-seeded
		// finance.yarnmaster.lookupmaster.* codes (K-34 fail-open closure).
		"/finance.v1.LookupMasterService/UpdateLookupMaster": "finance.yarnmaster.lookupmaster.update",
		"/finance.v1.LookupMasterService/DeleteLookupMaster": "finance.yarnmaster.lookupmaster.delete",

		// ProductGradeService — mutating RPCs guarded with the pre-seeded
		// finance.yarnmaster.productgrade.* codes (K-34 fail-open closure).
		"/finance.v1.ProductGradeService/UpdateProductGrade": "finance.yarnmaster.productgrade.update",
		"/finance.v1.ProductGradeService/DeleteProductGrade": "finance.yarnmaster.productgrade.delete",

		// RMGroupService — DeleteRMGroup guarded with the pre-seeded
		// finance.rmpricing.grouphead.delete code (K-34 fail-open closure).
		"/finance.v1.RMGroupService/DeleteRMGroup": "finance.rmpricing.grouphead.delete",
	}

	return permissions[fullMethod]
}
