-- IAM Service Database Migrations
-- 000087: Seed the MB recipe UNLOCK REQUEST permission + grant it to the MB roles
--
-- =============================================================================
-- WHY THIS MIGRATION EXISTS — user decision (2026-08)
-- =============================================================================
-- Verbatim decision:
--   "peminta tidak boleh menyetujui permintaannya sendiri, jadi harus ada
--    permission baru untuk user tertentu yg bisa unlock, akan tetapi user dengan
--    permission unlock bisa langsung menyetujui ketika dia sendiri yg request"
--
-- Until now ALL THREE unlock RPCs shared ONE code, seeded by 000083 (:27):
--   RequestUnlockMBHead / GrantUnlockMBHead / RejectUnlockMBHead
--     -> finance.mb.recipe.unlock
-- so anyone who could ASK for an unlock could also GRANT their own. This splits
-- ASKING from DECIDING:
--
--   finance.mb.recipe.unlockrequest  (NEW, this migration) -> RequestUnlockMBHead
--   finance.mb.recipe.unlock          (EXISTING, 000083)    -> Grant + Reject
--
-- The separation is PURELY permission-based. There is deliberately NO
-- "is this my own request?" identity check anywhere in the backend: an ordinary
-- requester cannot approve because they do not hold finance.mb.recipe.unlock,
-- while a holder of finance.mb.recipe.unlock who happens to request it himself
-- MAY approve immediately — which is exactly the second half of the decision.
--
-- ⛔ 000083 IS NOT EDITED. It has ALREADY BEEN APPLIED on the user's local
-- database, so its checksum/effect is frozen history; golang-migrate would never
-- re-run it anyway. Every change therefore lands here as a NEW forward migration.
-- The EXISTING grants of finance.mb.recipe.unlock (000083 §4a SUPER_ADMIN, §4d
-- MB_APPROVER) are deliberately left UNTOUCHED — those two are precisely the
-- roles that should keep the DECIDING power.
--
-- Role matrix for the NEW request permission — every tier may ASK:
--   SUPER_ADMIN  : yes        MB_DRAFTER   : yes
--   MB_VALIDATOR : yes        MB_APPROVER  : yes
-- mst_role has no parent/inherit column (000004), so all four are listed
-- EXPLICITLY; nothing is inherited. Total: 4 role_permissions rows.
--
-- Role codes verified to exist, NOT recalled from memory:
--   SUPER_ADMIN (seeds/main.go); MB_DRAFTER (000068:37), MB_APPROVER (000068:38),
--   MB_VALIDATOR (000068:39).
-- Table/column names verified against 000004_create_rbac_tables.up.sql:
--   assignment table = role_permissions           (000004:113)
--   role PK          = mst_role.role_id           (000004:11)
--   permission PK    = mst_permission.permission_id (000004:41)
--   uq_permission_code UNIQUE (permission_code)   (000004:54)
--   uq_role_permission UNIQUE (role_id, permission_id) (000004:121)
--
-- =============================================================================
-- ✅ RESOLVED — code shortened to 4 segments. THIS MIGRATION IS RUNNABLE.
-- =============================================================================
-- HISTORY, kept deliberately so the constraint is never re-discovered the hard
-- way: the orchestrator's FIRST choice was 'finance.mb.recipe.unlock.request'
-- with FIVE dot-separated segments. That code WOULD HAVE FAILED. Two independent
-- guards accept only FOUR segments:
--
--   (1) chk_permission_code_format  (000004:55), NEVER altered by any migration:
--         ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--       The trailing [a-z]+ contains no dot, so it cannot match "unlock.request";
--       the INSERT would raise a CHECK violation and abort the migration.
--       Evidence: all 390 distinct permission codes across every *.up.sql have
--       exactly 4 segments; there are ZERO 5-segment codes in the whole repo.
--
--   (2) the Go mirror of that regex, internal/domain/role/entity.go:27, enforced
--       by NewPermission() at :175. Seeding via raw SQL bypasses it, but an admin
--       later re-creating/editing this permission through the IAM API would hit
--       ErrInvalidPermissionCodeFormat.
--
-- RESOLUTION (orchestrator, 2026-08-26): CONCATENATE rather than widen.
--       'finance.mb.recipe.unlockrequest'   — 4 segments, ZERO DDL.
-- This is the very convention goapps-backend/CLAUDE.md §7 documents for
-- multi-word entities ('employeelevel', 'companymap') and that already produced
-- 'mbxsection' (000083) and 'spinfixedcost' (000082).
-- The REJECTED alternative was to keep 5 segments and widen BOTH guards (ALTER
-- the CHECK constraint here AND relax entity.go:27). That would break a
-- platform-wide invariant currently satisfied by 390 rows in order to save a
-- single dot — a bad trade, so it was not taken.
--
-- ⚠ FAIL-CLOSED COUPLING: the string here and the string mapped to
-- "/finance.v1.MBHeadService/RequestUnlockMBHead" in
-- finance/internal/delivery/grpc/auth_interceptor.go MUST be CHARACTER-IDENTICAL.
-- An RPC mapped to a code no role holds rejects EVERY caller — not some callers.
-- Verified identical by cross-repo grep at the time of writing.
--
-- action_type below is 'unlock': it IS in the live chk_permission_action
-- whitelist (36 values, set by 000081:31-36). 'request' is NOT in that whitelist,
-- so 'unlock' is used deliberately — the same non-fatal convention deviation
-- 000085 documented. Nothing authorizes on action_type, only on permission_code.
-- This migration contains ZERO DDL.
-- =============================================================================

-- =============================================================================
-- 1. PERMISSION — 1 row
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint, so a bare
-- ON CONFLICT (permission_code) target is valid here (same as 000083 / 000086).

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    (gen_random_uuid(), 'finance.mb.recipe.unlockrequest', 'Request MB Recipe Unlock', 'Request that a locked MB recipe be unlocked, stating a reason. Deciding the request requires finance.mb.recipe.unlock.', 'finance', 'mb', 'unlock', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. ROLE ASSIGNMENTS — 4 rows (every MB tier may ASK for an unlock)
-- =============================================================================
-- ⛔ Deliberately does NOT touch any grant of finance.mb.recipe.unlock: the
-- DECIDING power stays exactly where 000083 put it (SUPER_ADMIN + MB_APPROVER).

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code IN ('SUPER_ADMIN', 'MB_DRAFTER', 'MB_VALIDATOR', 'MB_APPROVER')
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code = 'finance.mb.recipe.unlockrequest'
ON CONFLICT (role_id, permission_id) DO NOTHING;
