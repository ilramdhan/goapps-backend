-- IAM Service Database Migrations
-- 000084: Move "MB Cross Section" menu under the "MB Costing" section
--
-- User request (2026-08-22): the page /finance/master/mb-cross-section works,
-- but the sidebar entry sits under the wrong parent.
--
-- 000083 seeded FINANCE_MB_CROSS_SECTION with
--   parent_id = 00000000-0000-0000-0002-000000000002  (FINANCE_MASTER, "Master")
-- It belongs under
--   parent_id = 00000000-0000-0000-0002-000000000021  (FINANCE_MB_SECTION, "MB Costing", seeded by 000069)
--
-- 000083 is already applied in every environment, so it is NOT edited — the
-- change ships as this forward migration instead.
--
-- menu_level stays 3: both FINANCE_MASTER and FINANCE_MB_SECTION are level-2
-- section headers (000009:42 and 000069:6-9 respectively).
--
-- sort_order 5: the existing level-3 children of FINANCE_MB_SECTION occupy
-- 1..4 (MB Recipe 1, MB Push-to-Head 2, MB Lusture 3, MB Param 4 — 000069),
-- so 5 appends it to the end of that group without colliding. The old value
-- (40) came from the FINANCE_MASTER 10..35 scale and is meaningless here.
--
-- menu_url is deliberately UNCHANGED: sibling MB Costing leaves already mix
-- URL prefixes (/finance/mb-recipe, /finance/master/mb-lusture,
-- /finance/master/mb-param), so /finance/master/mb-cross-section stays valid
-- and no frontend route folder has to move.
--
-- Idempotent: a plain UPDATE guarded on the target parent existing. Scoped to
-- the single menu_code seeded by 000083 — no other row is touched.

UPDATE mst_menu
SET parent_id  = '00000000-0000-0000-0002-000000000021',
    menu_level = 3,
    sort_order = 5,
    updated_by = 'seed',
    updated_at = NOW()
WHERE menu_code = 'FINANCE_MB_CROSS_SECTION'
  AND deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM mst_menu p
      WHERE p.menu_id = '00000000-0000-0000-0002-000000000021'
        AND p.deleted_at IS NULL
  );
