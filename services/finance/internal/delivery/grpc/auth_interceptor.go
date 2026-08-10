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
		"/finance.v1.UOMService/CreateUOM": "finance.master.uom.create",
		"/finance.v1.UOMService/GetUOM":    "finance.master.uom.view",
		"/finance.v1.UOMService/ListUOMs":  "finance.master.uom.view",
		"/finance.v1.UOMService/UpdateUOM": "finance.master.uom.update",
		"/finance.v1.UOMService/DeleteUOM": "finance.master.uom.delete",
		"/finance.v1.UOMService/ImportUOM": "finance.master.uom.create",
		"/finance.v1.UOMService/ExportUOM": "finance.master.uom.view",

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
		"/finance.v1.MBHeadService/CreateMBHead":           "finance.mb.head.create",
		"/finance.v1.MBHeadService/GetMBHead":              "finance.mb.head.view",
		"/finance.v1.MBHeadService/ListMBHeads":            "finance.mb.head.view",
		"/finance.v1.MBHeadService/UpdateMBHead":           "finance.mb.head.update",
		"/finance.v1.MBHeadService/DeleteMBHead":           "finance.mb.head.delete",
		"/finance.v1.MBHeadService/ExportMBHeads":          "finance.mb.head.view",
		"/finance.v1.MBHeadService/ImportMBHeads":          "finance.mb.head.create",
		"/finance.v1.MBHeadService/DownloadMBHeadTemplate": "finance.mb.head.view",
		"/finance.v1.MBHeadService/SubmitMBHead":           "finance.mb.head.submit",
		"/finance.v1.MBHeadService/ApproveMBHead":          "finance.mb.head.approve",
		"/finance.v1.MBHeadService/ValidateMBHead":         "finance.mb.head.validate",
		"/finance.v1.MBHeadService/UnApproveMBHead":        "finance.mb.head.unapprove",
		"/finance.v1.MBHeadService/RevokeMBHead":           "finance.mb.head.revoke",

		// MBSpinService (reuses finance.yarnmaster.mbspin.* seeded in iam mig 000057)
		"/finance.v1.MBSpinService/CreateMBSpin":           "finance.yarnmaster.mbspin.create",
		"/finance.v1.MBSpinService/GetMBSpin":              "finance.yarnmaster.mbspin.view",
		"/finance.v1.MBSpinService/ListMBSpins":            "finance.yarnmaster.mbspin.view",
		"/finance.v1.MBSpinService/UpdateMBSpin":           "finance.yarnmaster.mbspin.update",
		"/finance.v1.MBSpinService/DeleteMBSpin":           "finance.yarnmaster.mbspin.delete",
		"/finance.v1.MBSpinService/ExportMBSpins":          "finance.yarnmaster.mbspin.view",
		"/finance.v1.MBSpinService/ImportMBSpins":          "finance.yarnmaster.mbspin.create",
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
	}

	return permissions[fullMethod]
}
