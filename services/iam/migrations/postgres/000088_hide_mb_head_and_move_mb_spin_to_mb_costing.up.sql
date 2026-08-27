-- IAM Service Database Migrations
-- 000088: Hide "MB Head" menu; move "MB Spin" menu under "MB Costing"
--
-- Date: 2026-08-26
--
-- User request: MB Head has been superseded by MB Recipe, which runs on the
-- same backend. User was asked whether to (a) hide the menu only,
-- (b) also delete the old frontend page, or (c) also delete the backend.
-- User's answer: "sembunyikan menunya saja" (hide the menu only) -> option
-- (a) ONLY. This migration therefore touches ONLY mst_menu rows. It does
-- NOT delete any row, does NOT touch table mst_mb_head, does NOT remove any
-- RPC or backend handler, and does NOT remove any frontend file.
--
-- Change 1: Hide FINANCE_MB_HEAD
--   menu_id = 00000000-0000-0000-0003-000000000024, seeded by
--   000057_seed_yarn_master_menus.up.sql:77 with is_visible = TRUE.
--   Column verified against the mst_menu column list used by both
--   000057 (line 4-5) and 000069 (line 2-4): "... sort_order, is_visible,
--   is_active, created_by". We set is_visible = FALSE only; is_active is
--   left untouched (TRUE) since the row/data/backend are still fully live
--   for anyone hitting the URL/permission directly (e.g. MB Recipe reuses
--   a different permission code, finance.mb.head.view, so it is unaffected).
--   Permission finance.yarnmaster.mbhead.* stays as-is; access control is
--   keyed off menu_id, not parent_id, so this does not cascade to anything
--   else.
--
-- Change 2: Move FINANCE_MB_SPIN from "Yarn Master" to "MB Costing"
--   menu_id = 00000000-0000-0000-0003-000000000025, seeded by
--   000057_seed_yarn_master_menus.up.sql:78 with
--   parent_id = 00000000-0000-0000-0002-000000000016 (FINANCE_YARN_MASTER),
--   menu_level = 3, sort_order = 50.
--   New parent: 00000000-0000-0000-0002-000000000021 (FINANCE_MB_SECTION,
--   "MB Costing"), seeded by 000069_seed_mb_menus.up.sql:6-9.
--   menu_level stays 3: both FINANCE_YARN_MASTER (000057 header, level 2)
--   and FINANCE_MB_SECTION (000069:11, level 2) are level-2 section
--   headers, so a direct child of either is level 3 -- unchanged.
--   sort_order 6: current level-3 children of FINANCE_MB_SECTION occupy
--   1..4 (MB Recipe 1, MB Push-to-Head 2, MB Lusture 3, MB Param 4, from
--   000069) and 5 (MB Cross Section, moved in by 000084), so 6 appends
--   MB Spin to the end of that group without colliding. Verified by
--   reading 000069_seed_mb_menus.up.sql and
--   000084_move_mb_cross_section_to_mb_costing.up.sql directly.
--   menu_url is deliberately UNCHANGED (stays
--   /finance/yarn-master/mb-spins): per 000084's precedent, MB Costing
--   siblings already mix URL prefixes, so no frontend route folder needs
--   to move. Whether to later rename the URL to match its new parent is
--   an open product decision, NOT made here.
--
-- Idempotent: plain UPDATEs guarded by EXISTS on the referenced menu rows,
-- scoped to exactly one menu_code each. Following the pattern established
-- by 000084 (which may NOT be edited, per repo convention -- it already
-- shipped).

-- Change 1: hide MB Head
UPDATE mst_menu
SET is_visible = FALSE,
    updated_by = 'seed',
    updated_at = NOW()
WHERE menu_code = 'FINANCE_MB_HEAD'
  AND deleted_at IS NULL;

-- Change 2: move MB Spin under MB Costing
UPDATE mst_menu
SET parent_id  = '00000000-0000-0000-0002-000000000021',
    menu_level = 3,
    sort_order = 6,
    updated_by = 'seed',
    updated_at = NOW()
WHERE menu_code = 'FINANCE_MB_SPIN'
  AND deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM mst_menu p
      WHERE p.menu_id = '00000000-0000-0000-0002-000000000021'
        AND p.deleted_at IS NULL
  );
