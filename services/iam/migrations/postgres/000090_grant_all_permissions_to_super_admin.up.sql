-- IAM Service Database Migrations
-- 000090: Grant ALL existing permissions to role SUPER_ADMIN (explicit safety net).
--
-- Context: 000089 seeded the finance.master.shade.* permissions and granted them
-- ONLY to SUPER_ADMIN (000089 §5). Around the same time, the six ShadeService
-- RPCs were registered into the finance backend's permission map, so from now
-- on ONLY SUPER_ADMIN can use the Shade page — every other role is locked out
-- unless/until a follow-up migration grants Shade permissions to them too.
--
-- User decision (2026-08-27, verbatim): "setelah seed permission aktif berikan
-- semua permission ke role super admin dulu" — i.e. before deciding which other
-- roles get Shade (or any other still-ungranted permission), make sure
-- SUPER_ADMIN itself is never the one left short. This migration is
-- intentionally broader than Shade: it grants role SUPER_ADMIN every permission
-- that exists in mst_permission today, not just the five finance.master.shade.*
-- ones, so this class of gap (new permission registered in code before a seed
-- migration grants it to SUPER_ADMIN) cannot recur for SUPER_ADMIN specifically.
--
-- IMPORTANT — this is a belt-and-suspenders / explicit-grant measure, NOT a
-- functional fix by itself: finance's auth interceptor
-- (goapps-backend/services/finance/.../auth_interceptor.go, IsSuperAdmin())
-- already bypasses ALL permission checks for SUPER_ADMIN regardless of what
-- rows exist in role_permissions. So SUPER_ADMIN was never actually blocked
-- from Shade or anything else at runtime. This migration exists so that the
-- *data* (role_permissions) reflects what SUPER_ADMIN can already do in
-- practice, in case something ever starts consulting role_permissions instead
-- of (or in addition to) the code-level bypass, and so permission-picker /
-- audit UIs that read role_permissions show SUPER_ADMIN as fully granted.
--
-- Deliberately OUT OF SCOPE (user decision, not an oversight): permissions are
-- NOT granted here to any role other than SUPER_ADMIN. Which other roles (if
-- any) should get finance.master.shade.* — or any other still-ungranted
-- permission — is a separate decision the user explicitly deferred ("...dulu")
-- to a follow-up migration.
--
-- Verified facts (read from schema + sibling migrations, not guessed):
--   role table              = mst_role (role_id, role_code, is_active)        (000004:11-26)
--   permission table        = mst_permission (permission_id, permission_code,
--                              is_active)                                      (000004:41-60)
--   role<->permission table = role_permissions (id, role_id, permission_id,
--                              assigned_by), UNIQUE (role_id, permission_id)   (000004:113-124)
--   SUPER_ADMIN role_code   = 'SUPER_ADMIN' (exact case), used verbatim by every
--                             prior seed migration that targets it, e.g.
--                             000089:110, 000068, 000079, 000050.
--
-- Set-based, idempotent: ON CONFLICT DO NOTHING on the (role_id, permission_id)
-- unique constraint means re-running this migration (or running it after some
-- permissions already had a SUPER_ADMIN grant from an earlier seed) is a no-op
-- for rows that already exist.

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
