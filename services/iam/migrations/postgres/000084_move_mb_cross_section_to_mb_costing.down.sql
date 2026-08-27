-- Rollback 000084: put "MB Cross Section" back under FINANCE_MASTER ("Master")
-- with the exact values 000083 seeded (parent 0002-...0002, level 3, sort 40).
--
-- NOTE: the release flow for this repo is forward-only (decision K-14). This
-- .down.sql exists as a repo convention + emergency hatch and is not expected
-- to run in staging or production.

UPDATE mst_menu
SET parent_id  = '00000000-0000-0000-0002-000000000002',
    menu_level = 3,
    sort_order = 40,
    updated_by = 'seed',
    updated_at = NOW()
WHERE menu_code = 'FINANCE_MB_CROSS_SECTION'
  AND deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM mst_menu p
      WHERE p.menu_id = '00000000-0000-0000-0002-000000000002'
        AND p.deleted_at IS NULL
  );
