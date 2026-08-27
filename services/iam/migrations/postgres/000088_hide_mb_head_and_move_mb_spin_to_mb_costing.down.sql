-- Rollback 000088: unhide "MB Head" and put "MB Spin" back under
-- "Yarn Master" with the exact values 000057 originally seeded
-- (parent 0002-...0016, level 3, sort 50).
--
-- NOTE: this repo's release flow is forward-only (decision K-14, see
-- 000084's down file). This .down.sql exists as a repo convention +
-- emergency hatch and is not expected to run in staging or production.

-- Revert change 2: MB Spin back under Yarn Master
UPDATE mst_menu
SET parent_id  = '00000000-0000-0000-0002-000000000016',
    menu_level = 3,
    sort_order = 50,
    updated_by = 'seed',
    updated_at = NOW()
WHERE menu_code = 'FINANCE_MB_SPIN'
  AND deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM mst_menu p
      WHERE p.menu_id = '00000000-0000-0000-0002-000000000016'
        AND p.deleted_at IS NULL
  );

-- Revert change 1: unhide MB Head
UPDATE mst_menu
SET is_visible = TRUE,
    updated_by = 'seed',
    updated_at = NOW()
WHERE menu_code = 'FINANCE_MB_HEAD'
  AND deleted_at IS NULL;
