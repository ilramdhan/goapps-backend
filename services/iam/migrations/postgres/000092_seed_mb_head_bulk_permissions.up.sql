-- IAM Service Database Migrations
-- 000092: Seed finance.mb.head.bulk{unvalidate,submit,validate} permissions, SUPER_ADMIN only
--
-- Context: production has accumulated MB Head recipes that are all sitting in
-- VALIDATED status. To regenerate their downstream cost_product_master /
-- cost_route_* / CAPP / CPP / MB Spin data (after a formula/calc fix), an
-- admin needs to re-trigger the full validate lifecycle across many rows at
-- once: force-unvalidate -> submit -> validate. Bulk force-unvalidate
-- collapses the normal 2-step unlock-request flow (see 000087) into a single
-- one-shot action across many rows simultaneously, so — same precedent as
-- 000091's unrevoke permission — it is gated to a single, NEW, DEDICATED set
-- of permissions granted ONLY to SUPER_ADMIN. No other role (MB_DRAFTER/
-- MB_VALIDATOR/MB_APPROVER/FINANCE) receives these.
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   chk_permission_code_format = ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--   finance.mb.head.bulkunvalidate -- 4 lowercase segments, no underscores/hyphens. Verified against the regex.
--   finance.mb.head.bulksubmit     -- 4 lowercase segments, no underscores/hyphens. Verified against the regex.
--   finance.mb.head.bulkvalidate   -- 4 lowercase segments, no underscores/hyphens. Verified against the regex.
--
-- chk_permission_action WIDENED here: the live constraint (37 values, last
-- extended by 000091 with 'unrevoke') does NOT contain 'bulkunvalidate',
-- 'bulksubmit', or 'bulkvalidate' -- confirmed by reading the constraint
-- definition directly in 000091:31-37. This migration appends all three as
-- the 38th-40th allowed action_type values.
--
-- 000090 (grant-all-permissions-to-SUPER_ADMIN) is a one-time blanket grant
-- already applied against the permissions that existed at that time -- it does
-- NOT retroactively cover permissions created by later migrations, so this
-- migration grants SUPER_ADMIN explicitly rather than relying on 000090.

-- =============================================================================
-- 0. EXTEND chk_permission_action -- allow 'bulkunvalidate', 'bulksubmit', 'bulkvalidate'
-- =============================================================================
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke','bulkunvalidate','bulksubmit','bulkvalidate'
  ));

-- =============================================================================
-- 1. PERMISSIONS -- 3 rows
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint, so a bare
-- ON CONFLICT (permission_code) target is valid here (same as 000091).

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    (gen_random_uuid(), 'finance.mb.head.bulkunvalidate', 'Bulk Force-Unvalidate MB Head', 'Force-unvalidate many validated MB head recipes at once, bypassing the normal 2-step unlock-request flow (Super Admin only)', 'finance', 'mb', 'bulkunvalidate', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.mb.head.bulksubmit', 'Bulk Submit MB Head', 'Submit many draft MB head recipes for validation at once, as part of the bulk lifecycle regenerate flow (Super Admin only)', 'finance', 'mb', 'bulksubmit', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.mb.head.bulkvalidate', 'Bulk Validate MB Head', 'Validate many submitted MB head recipes at once, regenerating downstream cost_product_master/cost_route_*/CAPP/CPP/MB Spin data (Super Admin only)', 'finance', 'mb', 'bulkvalidate', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. ROLE GRANTS -- SUPER_ADMIN ONLY
-- =============================================================================
-- Deliberately no menu_permissions entries and no other role: bulk lifecycle
-- regenerate is not a menu-visible feature toggle, it is a targeted Super
-- Admin operational tool. MB_DRAFTER/MB_VALIDATOR/MB_APPROVER/FINANCE do NOT
-- receive these permissions.

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND r.is_active = true
  AND p.is_active = true
  AND p.permission_code IN ('finance.mb.head.bulkunvalidate', 'finance.mb.head.bulksubmit', 'finance.mb.head.bulkvalidate')
ON CONFLICT (role_id, permission_id) DO NOTHING;
