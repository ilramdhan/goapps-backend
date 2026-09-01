-- IAM Service Database Migrations
-- 000091: Seed finance.mb.head.unrevoke permission, SUPER_ADMIN only
--
-- User decision (2026-08-31, Opsi A): Revoke stays a removed feature (no
-- entrance is reopened into REVOKED — canRevoke() always reports false), but a
-- REVOKED mst_mb_head row was otherwise frozen forever with no way out. This
-- adds a single, NEW, DEDICATED permission gating the new REVOKED -> DRAFT
-- "Unrevoke" transition (UnrevokeMBHead RPC) — granted ONLY to SUPER_ADMIN,
-- unlike ReturnMBHeadToDraft (REJECTED -> DRAFT) which reuses the existing
-- finance.mb.head.submit permission. Unrevoke is deliberately NOT reusable by
-- a plain author: only Super Admin may pull a row back out of REVOKED.
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   chk_permission_code_format = ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--   finance.mb.head.unrevoke -- 4 lowercase segments, no underscores/hyphens. Verified against the regex.
--
-- chk_permission_action WIDENED here: the live constraint (36 values, last
-- extended by 000081 with 'sync') does NOT contain 'unrevoke' -- confirmed by
-- reading the constraint definition directly in 000081:31-36. This migration
-- appends 'unrevoke' as the 37th allowed action_type.
--
-- 000090 (grant-all-permissions-to-SUPER_ADMIN) is a one-time blanket grant
-- already applied against the permissions that existed at that time -- it does
-- NOT retroactively cover permissions created by later migrations, so this
-- migration grants SUPER_ADMIN explicitly rather than relying on 000090.

-- =============================================================================
-- 0. EXTEND chk_permission_action -- allow 'unrevoke'
-- =============================================================================
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke'
  ));

-- =============================================================================
-- 1. PERMISSION -- 1 row
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint, so a bare
-- ON CONFLICT (permission_code) target is valid here (same as 000083).

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    (gen_random_uuid(), 'finance.mb.head.unrevoke', 'Unrevoke MB Head', 'Return a revoked MB head recipe back to draft for editing and resubmission (Super Admin only)', 'finance', 'mb', 'unrevoke', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. ROLE GRANT -- SUPER_ADMIN ONLY
-- =============================================================================
-- Deliberately no menu_permissions entry and no other role: Unrevoke is not a
-- menu-visible feature toggle, it is a targeted Super Admin escape hatch for a
-- state that would otherwise be permanently stuck. MB_DRAFTER/MB_VALIDATOR/
-- MB_APPROVER/FINANCE do NOT receive this permission.

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND r.is_active = true
  AND p.is_active = true
  AND p.permission_code = 'finance.mb.head.unrevoke'
ON CONFLICT (role_id, permission_id) DO NOTHING;
