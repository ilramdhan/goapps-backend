-- IAM Service Database Migrations
-- 000085: Seed MB Dozing (LDR calculator) permissions + role assignments
--
-- User decision K-17 (gate G21-DOZ-PERM), option (ii): create BRAND NEW
-- permissions for the MB dozing LDR calculator instead of piggy-backing on
-- finance.mb.head.view / .update. Piggy-backing was explicitly REJECTED — it
-- is exactly the mistake recorded as lesson G17-FAKTOR-PERM.
--
-- WHY THIS MATTERS: services/finance/internal/delivery/grpc/auth_interceptor.go
-- lines 331-334 is FAIL-OPEN — an RPC with no entry in the permission map is
-- allowed through on authentication alone, with NO permission check. Until
-- these two codes exist AND the RPCs are mapped to them, the dozing calculator
-- is effectively unprotected.
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   chk_permission_code_format = ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--   Both codes below were checked against this regex before writing:
--   `dozing` is a single lowercase run, so no concatenation is needed (unlike
--   `mbxsection` in 000083 or `spinfixedcost` in 000082).
--
-- =============================================================================
-- !!! OPEN DECISION GATE — action_type FOR THE `.calculate` PERMISSION !!!
-- =============================================================================
-- chk_permission_action does NOT contain 'calculate'. The live whitelist has 36
-- values (set by 000081, which extended 000068's 35 with 'sync'); it contains
-- 'recalculate', 'execute', 'trigger' and 'preview', but NOT 'calculate'.
--
-- This migration is mandated to contain ZERO DDL (pure seed / INSERT only), so
-- it CANNOT widen the constraint. Every one of the ~200 existing permission
-- rows has action_type equal to the last segment of its permission_code
-- (verified programmatically across all *.up.sql — 0 mismatches), so this row
-- is the FIRST deliberate deviation from that convention:
--
--   permission_code 'finance.mb.dozing.calculate'  ->  action_type 'execute'
--
-- This is SAFE for authorization: nothing authorizes on action_type. The
-- interceptor and both permission repositories resolve exclusively by
-- permission_code (see permission_repository.go:100,291 and
-- user_permission_repository.go:66,104). action_type is descriptive metadata.
--
-- USER MUST CHOOSE one of:
--   (A) keep 'execute' as written here (no DDL, convention broken once); or
--   (B) authorize a separate DDL migration adding 'calculate' to
--       chk_permission_action, then a follow-up UPDATE of this row.
-- Nothing here is guessed — the alternative required DDL that was forbidden.
-- =============================================================================

-- =============================================================================
-- 1. PERMISSIONS — 2 rows
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint (000004:54),
-- so a bare ON CONFLICT (permission_code) target is valid (same as 000082/000083).

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    (gen_random_uuid(), 'finance.mb.dozing.calculate', 'Calculate MB Dozing', 'Run the MB dozing LDR calculator (SCALE / XSECTION modes)', 'finance', 'mb', 'execute', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.mb.dozing.preview',   'Preview MB Dozing',   'View the MB dozing impact preview before applying results',  'finance', 'mb', 'preview', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. ROLE ASSIGNMENTS
-- =============================================================================
-- ⚠ FINANCE_ADMIN DOES NOT EXIST in this system and is deliberately NOT
--   referenced (user decision K-12). The roles that DO exist are:
--   SUPER_ADMIN, MB_DRAFTER, MB_VALIDATOR, MB_APPROVER, FINANCE.
--
-- ⚠⚠ WARNING — SUPER_ADMIN IS NOT SEEDED BY ANY MIGRATION. It is created by Go
--    code, services/iam/seeds/main.go:105. On a database where `make seed` has
--    NOT been run, the SUPER_ADMIN block below matches zero rows and inserts
--    NOTHING, SILENTLY, while `make migrate-up` still reports success. If you
--    run this on a fresh DB, run `make seed` first, or re-run the verification
--    SELECTs afterwards and confirm the SUPER_ADMIN row count is 2, not 0.
--    (The same is true for MB_* roles if 000068 has not been applied.)
--
-- Mapping rationale — derived from how the ANALOGOUS finance.mb.head.*
-- permissions are already distributed (000068_mb_roles_and_permissions.up.sql),
-- NOT invented:
--   finance.mb.head.view   -> MB_DRAFTER, MB_VALIDATOR, MB_APPROVER, SUPER_ADMIN
--   finance.mb.head.update -> MB_DRAFTER, SUPER_ADMIN            <-- narrowest
--
--   .calculate mutates recipe dosing figures, so it is the behavioural twin of
--   finance.mb.head.update -> MB_DRAFTER + SUPER_ADMIN.
--   .preview is read-only, so finance.mb.head.view would suggest all three MB
--   roles. That choice is genuinely AMBIGUOUS, so per instruction the MOST
--   CONSERVATIVE (fewest rights) option is written: preview is granted only to
--   the roles that can actually act on it, i.e. MB_DRAFTER + SUPER_ADMIN.
--   Note the nearest precedent agrees that preview is not universal:
--   finance.mb.pushtohead.preview (000068) went to MB_VALIDATOR ALONE.
--
-- >>> NEEDS USER REVIEW: whether MB_VALIDATOR and MB_APPROVER should also
-- >>> receive finance.mb.dozing.preview. This migration says NO (conservative).
--
-- FINANCE receives nothing and therefore has no INSERT block at all — same
-- treatment as in 000083.
--
-- mst_role has NO parent/inherit column (000004), and the interceptor resolves
-- role_permissions rows directly, so every grant is listed EXPLICITLY.
-- Expected total: SUPER_ADMIN 2 + MB_DRAFTER 2 = 4 role_permissions rows.
--
-- Idempotency: pure ON CONFLICT (role_id, permission_id) DO NOTHING against
-- uq_role_permission (000004:126). NO silent-skip guard of the
-- `WHERE (SELECT ...) IS NOT NULL` form is used anywhere here — that pattern is
-- the known defect in 000480 and is deliberately avoided.

-- 2a. SUPER_ADMIN — both permissions
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.mb.dozing.calculate',
        'finance.mb.dozing.preview'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 2b. MB_DRAFTER — both permissions (mirrors its finance.mb.head.update grant)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_DRAFTER'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.mb.dozing.calculate',
        'finance.mb.dozing.preview'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;
