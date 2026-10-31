-- IAM Service Database Migrations
-- 000099: Restructure the MB roles (MB Recipe, MB Spin, MB Batch, Push-to-Head) + add MB_ADMIN
--
-- User decision (2026-10-31), after a trace of 000068/000083/000085/000087 against the
-- finance auth_interceptor showed the seeded MB roles could not actually be used on
-- their own:
--   * none of them held finance.module.root.view, so the whole "Finance" sidebar root
--     (000009:86) — and therefore MB Costing > MB Recipe — was hidden from them;
--   * none of them could open MB Spin (finance.yarnmaster.mbspin.* was SUPER_ADMIN-only, 000057);
--   * the MB Recipe page's Cross Section dropdown and Cost tab call
--     finance.master.mbxsection(.factor).view and finance.cost.{result,history}.view,
--     which no MB role held.
--
-- Agreed matrix ("BASE" = every MB role, MB_ADMIN included):
--   BASE        : finance.module.root.view, finance.mb.head.view, finance.mb.recipe.export,
--                 finance.mb.recipe.unlockrequest, finance.master.{mblusture,mbparam,
--                 mbxsection,mbxsectionfactor}.view, finance.yarnmaster.mbspin.view,
--                 finance.cost.result.view, finance.cost.history.view
--   MB_DRAFTER  : BASE + mb.head.{create,update,delete,submit}, mb.dozing.{calculate,preview},
--                 yarnmaster.mbspin.duplicate
--                 (head.delete is DRAFT-only, enforced by the finance DeleteHandler.)
--   MB_APPROVER : BASE + mb.head.{approve,reject}, mb.recipe.unlock (grant/reject unlock)
--   MB_VALIDATOR: BASE + mb.pushtohead.{preview,execute}, mb.batch.trigger
--   MB_ADMIN    : (NEW) every grant above + CRUD of MB Spin and of the MB masters
--                 (lusture, param, cross section, cross section factor) + mb.head.validate
--                 (moves legacy APPROVED rows forward).
--   ⛔ Still SUPER_ADMIN-only, NOT given to MB_ADMIN: mb.head.unrevoke (000091),
--      mb.head.bulk{unvalidate,submit,validate} (000092).
--
-- REVOKED from the old roles (dead features, user decision 2026-08-26 — no RPC/button
-- uses them any more). The permissions themselves are NOT deleted, SUPER_ADMIN keeps them:
--   MB_APPROVER : mb.head.unapprove, mb.recipe.lock, yarnmaster.mbspin.duplicate
--   MB_VALIDATOR: mb.head.revoke, mb.head.validate, mb.recipe.lock, yarnmaster.mbspin.duplicate
-- finance.yarnmaster.mbspin.duplicate is KEPT on MB_DRAFTER/MB_ADMIN: it is reserved for the
-- planned "duplicate MB Spin with the old spin as head" feature (DuplicateMBSpin currently
-- maps to mbspin.create in auth_interceptor.go; remap it when that feature lands).
--
-- mst_role has no inherit column (000004); every grant is listed explicitly.
-- No DDL. Idempotent (ON CONFLICT DO NOTHING / plain DELETE).

-- =============================================================================
-- 0. finance.module.root.view is created by seeds/main.go, not by a migration.
--    Insert it defensively so a fresh DB (migrate before seed) does not make the
--    grants below silently skip it. Same row the seeder writes; DO NOTHING if present.
-- =============================================================================
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    (gen_random_uuid(), 'finance.module.root.view', 'Access Finance Module', 'Access Finance Module', 'finance', 'module', 'view', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 1. NEW ROLE — MB_ADMIN
-- =============================================================================
INSERT INTO mst_role (role_id, role_code, role_name, description, is_system, is_active, created_by) VALUES
    (gen_random_uuid(), 'MB_ADMIN', 'MB Admin', 'Full MB recipe workflow, MB Spin, MB Batch, Push-to-Head and MB master data (not unrevoke/bulk)', FALSE, TRUE, 'seed')
ON CONFLICT (role_code) DO NOTHING;

-- =============================================================================
-- 2. GUARD — every permission code used below must exist and be active,
--    otherwise the CROSS JOIN grants would match zero rows silently.
-- =============================================================================
DO $$
DECLARE
    missing text;
BEGIN
    SELECT string_agg(c.code, ', ') INTO missing
    FROM (VALUES
        ('finance.module.root.view'), ('finance.mb.head.view'), ('finance.mb.recipe.export'),
        ('finance.mb.recipe.unlockrequest'), ('finance.master.mblusture.view'), ('finance.master.mbparam.view'),
        ('finance.master.mbxsection.view'), ('finance.master.mbxsectionfactor.view'),
        ('finance.yarnmaster.mbspin.view'), ('finance.cost.result.view'), ('finance.cost.history.view'),
        ('finance.mb.head.create'), ('finance.mb.head.update'), ('finance.mb.head.delete'), ('finance.mb.head.submit'),
        ('finance.mb.dozing.calculate'), ('finance.mb.dozing.preview'), ('finance.yarnmaster.mbspin.duplicate'),
        ('finance.mb.head.approve'), ('finance.mb.head.reject'), ('finance.mb.recipe.unlock'),
        ('finance.mb.pushtohead.preview'), ('finance.mb.pushtohead.execute'), ('finance.mb.batch.trigger'),
        ('finance.mb.head.validate'),
        ('finance.yarnmaster.mbspin.create'), ('finance.yarnmaster.mbspin.update'), ('finance.yarnmaster.mbspin.delete'),
        ('finance.master.mblusture.create'), ('finance.master.mblusture.update'), ('finance.master.mblusture.delete'),
        ('finance.master.mbparam.create'), ('finance.master.mbparam.update'), ('finance.master.mbparam.delete'),
        ('finance.master.mbxsection.create'), ('finance.master.mbxsection.update'), ('finance.master.mbxsection.delete'),
        ('finance.master.mbxsectionfactor.create'), ('finance.master.mbxsectionfactor.update'), ('finance.master.mbxsectionfactor.delete')
    ) AS c(code)
    WHERE NOT EXISTS (
        SELECT 1 FROM mst_permission p WHERE p.permission_code = c.code AND p.is_active = TRUE
    );
    IF missing IS NOT NULL THEN
        RAISE EXCEPTION 'migration 000099: missing/inactive permissions: %', missing;
    END IF;
END
$$;

-- =============================================================================
-- 3. GRANTS — explicit (role_code, permission_code) matrix
-- =============================================================================
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM (
    -- BASE × every MB role
    SELECT role_code, permission_code
    FROM (VALUES ('MB_DRAFTER'), ('MB_APPROVER'), ('MB_VALIDATOR'), ('MB_ADMIN')) AS rl(role_code)
    CROSS JOIN (VALUES
        ('finance.module.root.view'), ('finance.mb.head.view'), ('finance.mb.recipe.export'),
        ('finance.mb.recipe.unlockrequest'), ('finance.master.mblusture.view'), ('finance.master.mbparam.view'),
        ('finance.master.mbxsection.view'), ('finance.master.mbxsectionfactor.view'),
        ('finance.yarnmaster.mbspin.view'), ('finance.cost.result.view'), ('finance.cost.history.view')
    ) AS pm(permission_code)
    UNION ALL
    -- DRAFTER tier (MB_DRAFTER + MB_ADMIN)
    SELECT role_code, permission_code
    FROM (VALUES ('MB_DRAFTER'), ('MB_ADMIN')) AS rl(role_code)
    CROSS JOIN (VALUES
        ('finance.mb.head.create'), ('finance.mb.head.update'), ('finance.mb.head.delete'), ('finance.mb.head.submit'),
        ('finance.mb.dozing.calculate'), ('finance.mb.dozing.preview'), ('finance.yarnmaster.mbspin.duplicate')
    ) AS pm(permission_code)
    UNION ALL
    -- APPROVER tier (MB_APPROVER + MB_ADMIN)
    SELECT role_code, permission_code
    FROM (VALUES ('MB_APPROVER'), ('MB_ADMIN')) AS rl(role_code)
    CROSS JOIN (VALUES
        ('finance.mb.head.approve'), ('finance.mb.head.reject'), ('finance.mb.recipe.unlock')
    ) AS pm(permission_code)
    UNION ALL
    -- VALIDATOR tier (MB_VALIDATOR + MB_ADMIN)
    SELECT role_code, permission_code
    FROM (VALUES ('MB_VALIDATOR'), ('MB_ADMIN')) AS rl(role_code)
    CROSS JOIN (VALUES
        ('finance.mb.pushtohead.preview'), ('finance.mb.pushtohead.execute'), ('finance.mb.batch.trigger')
    ) AS pm(permission_code)
    UNION ALL
    -- MB_ADMIN only
    SELECT 'MB_ADMIN', permission_code
    FROM (VALUES
        ('finance.mb.head.validate'),
        ('finance.yarnmaster.mbspin.create'), ('finance.yarnmaster.mbspin.update'), ('finance.yarnmaster.mbspin.delete'),
        ('finance.master.mblusture.create'), ('finance.master.mblusture.update'), ('finance.master.mblusture.delete'),
        ('finance.master.mbparam.create'), ('finance.master.mbparam.update'), ('finance.master.mbparam.delete'),
        ('finance.master.mbxsection.create'), ('finance.master.mbxsection.update'), ('finance.master.mbxsection.delete'),
        ('finance.master.mbxsectionfactor.create'), ('finance.master.mbxsectionfactor.update'), ('finance.master.mbxsectionfactor.delete')
    ) AS pm(permission_code)
) AS m
JOIN mst_role r ON r.role_code = m.role_code AND r.is_active = TRUE
JOIN mst_permission p ON p.permission_code = m.permission_code AND p.is_active = TRUE
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- =============================================================================
-- 4. REVOKE dead-feature grants from the old roles (SUPER_ADMIN untouched)
-- =============================================================================
DELETE FROM role_permissions rp
USING mst_role r, mst_permission p,
    (VALUES
        ('MB_APPROVER',  'finance.mb.head.unapprove'),
        ('MB_APPROVER',  'finance.mb.recipe.lock'),
        ('MB_APPROVER',  'finance.yarnmaster.mbspin.duplicate'),
        ('MB_VALIDATOR', 'finance.mb.head.revoke'),
        ('MB_VALIDATOR', 'finance.mb.head.validate'),
        ('MB_VALIDATOR', 'finance.mb.recipe.lock'),
        ('MB_VALIDATOR', 'finance.yarnmaster.mbspin.duplicate')
    ) AS x(role_code, permission_code)
WHERE rp.role_id = r.role_id
  AND rp.permission_id = p.permission_id
  AND r.role_code = x.role_code
  AND p.permission_code = x.permission_code;
