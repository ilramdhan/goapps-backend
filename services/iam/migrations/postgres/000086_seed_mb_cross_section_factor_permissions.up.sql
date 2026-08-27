-- IAM Service Database Migrations
-- 000086: Seed MB Cross Section FACTOR (conversion factor) CRUD permissions
--         + role assignments
--
-- User decision K-21 (gate G17-FAKTOR-PERM, 2026-08-22), VERBATIM: CRUD of the
-- cross-section factor table uses BRAND NEW, SEPARATE permissions. It MUST NOT
-- piggy-back on the 4 existing finance.master.mbxsection.* master permissions.
-- Rationale recorded by the user: consistent with K-17 one round earlier (which
-- rejected piggy-backing for dozing as "repeating exactly the G17 lesson").
-- Conversion factors feed the LDR calculation, so the right to change them must
-- differ from the right to change the master CODE list.
--
-- 🔴 FINDING (2026-08-22): the finance interceptor ALREADY piggy-backed the 5
-- factor RPCs onto the master permissions —
--   services/finance/internal/delivery/grpc/auth_interceptor.go:436-440
-- mapped /finance.v1.MbCrossSectionFactorService/* to
-- finance.master.mbxsection.{create,update,delete,view}. That is precisely the
-- arrangement K-21 rejects. Those 5 lines are re-pointed to the codes below in
-- the same change set as this migration.
--
-- WHY THIS MATTERS: auth_interceptor.go:332 is FAIL-OPEN — `if required == ""`
-- lets an unmapped RPC through on authentication alone, with NO permission
-- check. So the interceptor map and this migration must land together.
--
-- =============================================================================
-- NAMING — DERIVED FROM THE EXISTING PATTERN, NOT INVENTED
-- =============================================================================
-- Existing master cross-section permissions (000083:33-36):
--   finance.master.mbxsection.view / .create / .update / .delete
--   service_name 'finance', module_name 'master'
-- Only the ENTITY segment changes here, master -> factor variant:
--   finance.master.mbxsectionfactor.view / .create / .update / .delete
--
-- chk_permission_code_format (000004:55):
--   ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- No underscore/hyphen is legal inside a segment, so the entity segment is the
-- concatenated run `mbxsectionfactor` — the same concatenation rule that
-- produced `mbxsection` (000083) and `spinfixedcost` (000082). Longest code is
-- 'finance.master.mbxsectionfactor.create' = 38 chars, well inside
-- permission_code VARCHAR(100).
--
-- action_type: view / create / update / delete. All four are in the live
-- chk_permission_action whitelist (36 values, set by 000081; originals from
-- 000004). Each equals the last segment of its permission_code, so the
-- project-wide convention (0 deviations across ~200 rows) holds here — unlike
-- 000085, which had to substitute 'execute' for a non-whitelisted 'calculate'.
-- This migration therefore contains ZERO DDL; the constraint is NOT widened.
--
-- Verified table/column names by re-reading 000004_create_rbac_tables.up.sql
-- (NOT from memory — this project has been bitten 3x by guessed names):
--   assignment table = role_permissions   (000004:113)   NOT mst_role_permission
--   role PK          = mst_role.role_id   (000004:11)    NOT id
--   permission PK    = mst_permission.permission_id (000004:41)  NOT id
--   uq_permission_code UNIQUE (permission_code)    (000004:54)
--   uq_role_permission UNIQUE (role_id, permission_id) (000004:121)

-- =============================================================================
-- 1. PERMISSIONS — 4 rows
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint, so a bare
-- ON CONFLICT (permission_code) target is valid — idempotent re-runs.

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    (gen_random_uuid(), 'finance.master.mbxsectionfactor.view',   'View MB Cross Section Factor',   'View MB cross section conversion factors (directed pairs)',            'finance', 'master', 'view',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.master.mbxsectionfactor.create', 'Create MB Cross Section Factor', 'Create an MB cross section conversion factor — affects LDR results',   'finance', 'master', 'create', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.master.mbxsectionfactor.update', 'Update MB Cross Section Factor', 'Update an MB cross section conversion factor — affects LDR results',   'finance', 'master', 'update', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.master.mbxsectionfactor.delete', 'Delete MB Cross Section Factor', 'Delete an MB cross section conversion factor — affects LDR results',   'finance', 'master', 'delete', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 1b. LOUD PRECONDITION CHECK — NO SILENT SKIP
-- =============================================================================
-- The banned pattern `WHERE (SELECT ...) IS NOT NULL` skips rows WITHOUT error
-- and leaves `make migrate-up` green. It is used NOWHERE in this file. Instead,
-- if the 4 permissions are not present and active after the INSERT (e.g. a
-- pre-existing row with is_active = false that ON CONFLICT DO NOTHING left
-- untouched, which would make every role grant below match zero rows), this
-- migration FAILS LOUDLY and rolls back.
DO $$
DECLARE
    n integer;
BEGIN
    SELECT count(*) INTO n
    FROM mst_permission
    WHERE is_active = true
      AND permission_code IN (
          'finance.master.mbxsectionfactor.view',
          'finance.master.mbxsectionfactor.create',
          'finance.master.mbxsectionfactor.update',
          'finance.master.mbxsectionfactor.delete'
      );
    IF n <> 4 THEN
        RAISE EXCEPTION
            'migration 000086: expected 4 active finance.master.mbxsectionfactor.* permissions, found %. Role grants would have matched zero rows silently.', n;
    END IF;
END
$$;

-- =============================================================================
-- 2. ROLE ASSIGNMENTS
-- =============================================================================
-- ⚠ FINANCE_ADMIN DOES NOT EXIST in this system and is deliberately NOT
--   referenced (user decision K-12). Existing roles: SUPER_ADMIN, MB_DRAFTER,
--   MB_VALIDATOR, MB_APPROVER, FINANCE.
--
-- ⚠⚠ WARNING — SUPER_ADMIN IS NOT SEEDED BY ANY MIGRATION. It is created by Go
--    code, services/iam/seeds/main.go:105. On a database where `make seed` has
--    NOT been run, the block below matches zero rows and inserts NOTHING,
--    SILENTLY, while `make migrate-up` still reports success. On a fresh DB run
--    `make seed` first, then re-run the verification SELECT and confirm the
--    SUPER_ADMIN row count is 4, not 0. (Same caveat for MB_* roles if 000068
--    has not been applied.)
--
-- MATRIX SEEDED HERE — deliberately the MOST CONSERVATIVE option:
--   SUPER_ADMIN : all 4          (K-12: SUPER_ADMIN gets everything)
--   every other role : NOTHING
--
-- Why SUPER_ADMIN-only rather than the dozing precedent (K-20: SUPER_ADMIN +
-- MB_DRAFTER)? Two reasons, both from existing decisions rather than invention:
--   1. The DIRECT analogue — the 4 finance.master.mbxsection.* master
--      permissions — is explicitly SUPER_ADMIN-only: "The 4 MB Cross Section
--      master permissions stay SUPER_ADMIN-only by decision — deliberately NOT
--      granted to any MB role" (000083_seed_mb_recipe_workflow_permissions
--      .up.sql:91-92). Factors sit in the same master module and screen.
--   2. Conversion factors are MORE sensitive than the dozing calculator: dozing
--      computes, factors are master data that every LDR computation reads.
--
-- >>> OPEN USER DECISION GATE: whether MB_DRAFTER (or MB_VALIDATOR /
-- >>> MB_APPROVER) should receive finance.master.mbxsectionfactor.view, or the
-- >>> full CRUD set. This migration grants them NOTHING. Nothing is guessed —
-- >>> widening requires a user decision and a follow-up migration.
--
-- mst_role has NO parent/inherit column (000004) and the interceptor resolves
-- role_permissions rows directly, so every grant is listed EXPLICITLY.
-- Expected total: SUPER_ADMIN 4 = 4 role_permissions rows.
-- Idempotent via ON CONFLICT (role_id, permission_id) DO NOTHING.

-- 2a. SUPER_ADMIN — all 4
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.master.mbxsectionfactor.view',
        'finance.master.mbxsectionfactor.create',
        'finance.master.mbxsectionfactor.update',
        'finance.master.mbxsectionfactor.delete'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- =============================================================================
-- 3. MENU — DELIBERATELY NONE. OPEN GATE.
-- =============================================================================
-- No mst_menu row and no menu_permissions row is created here, and this is a
-- judgement call the user must confirm rather than something guessed:
--   * The factors have NO dashboard page of their own. Only BFF API routes
--     exist (goapps-frontend/src/app/api/v1/finance/master/mb-cross-section-factor);
--     the single UI page is .../(dashboard)/finance/master/mb-cross-section,
--     served by the existing FINANCE_MB_CROSS_SECTION menu (000083:53, moved
--     under FINANCE_MB_SECTION "MB Costing" by 000084).
--   * Linking finance.master.mbxsectionfactor.view to that EXISTING menu would
--     LOOSEN it, not tighten it: menu_repository.go:349-351 shows
--     menu_permissions is OR-semantics ("has at least ONE required
--     permission"), so an extra link makes the menu visible to anyone holding
--     EITHER permission. Since the seeded matrix is SUPER_ADMIN-only that adds
--     no one today, but it would silently widen menu visibility the moment the
--     factor view permission is granted to another role.
--
-- >>> OPEN USER DECISION GATE: leave the menu gated on
-- >>> finance.master.mbxsection.view alone (as written), or also link
-- >>> finance.master.mbxsectionfactor.view to FINANCE_MB_CROSS_SECTION?

-- =============================================================================
-- 4. VERIFICATION (read-only — run manually, NOT part of the migration)
-- =============================================================================
-- SELECT permission_code, action_type, is_active FROM mst_permission
--  WHERE permission_code LIKE 'finance.master.mbxsectionfactor.%'
--  ORDER BY permission_code;             -- expect 4 rows
--
-- SELECT r.role_code, p.permission_code
--   FROM role_permissions rp
--   JOIN mst_role r       ON r.role_id       = rp.role_id
--   JOIN mst_permission p ON p.permission_id = rp.permission_id
--  WHERE p.permission_code LIKE 'finance.master.mbxsectionfactor.%'
--  ORDER BY r.role_code, p.permission_code;   -- expect 4 rows, all SUPER_ADMIN
