-- 000507: One-time backfill of the root MB Spin's and linked cost_product_master's shade columns
-- from their parent MB Head, closing the shade counterpart of the 31 Aug production bug (see
-- 000506 for the LDR half of the same incident).
--
-- Background: before this fix, mst_mb_spin's shade columns (mbs_shade_code/mbs_shade_name/
-- mbs_cross_section/mbs_cc) and cost_product_master's (cpm_shade_code/cpm_shade_name) were only
-- ever seeded once, at the very first Validate of a recipe (mbBuildAutoGenSpin /
-- mbInsertCostProductMaster, mb_autogen_repository.go). Every later re-validate went through
-- regenerateCostProductRMs, which never touched either table's shade columns — only
-- syncRootSpinLDRFromHead's LDR sync existed. Recipes whose shade (Master Shade dropdown) was
-- edited and re-validated after the first validate were left showing a stale shade on both the MB
-- Recipe/MB Spin page and the Master Product MB record. The application-code half of this fix
-- (syncRootSpinShadeFromHead + syncCostProductMasterShadeFromHead, mb_autogen_repository.go) makes
-- every future re-validate keep these in sync going forward; this migration catches up every row
-- that already drifted before the fix shipped.
--
-- Column names verified directly against the migrations that created them (and against the Go
-- getters that actually read/write them — mbh_shade_code, NOT mbh_code, is the real source; see
-- mbhead.Entity.ShadeCode()/mb_head_repository.go's Create/Update/selectCols):
--   * mst_mb_head:  mbh_id, mbh_shade_code, mbh_shade_name, mbh_cross_section  (000445)
--   * mst_mb_spin:  mbs_mbh_id, mbs_parent_spin_id, mbs_shade_code, mbs_shade_name,
--                    mbs_cross_section, mbs_cc                                  (000389/000404/000496)
--   * cost_product_master: cpm_product_sys_id, cpm_shade_code (000106), cpm_shade_name (000375)
--
-- Deliberately NO "is-locked/is-actual" guard here, unlike 000506's mbs_ldr_is_actual guard: shade
-- has no such lock column and is business-confirmed to always follow the recipe.
--
-- Guards:
--   * mbs_parent_spin_id IS NULL  — root spins only, never a Duplicate MB Spin clone.
--   * deleted_at IS NULL          — never touch a soft-deleted spin.
--   * head shade code/name IS NOT NULL — never write NULL over an existing value (recipes with no
--                                    shade at all are left untouched, mirroring 000506's
--                                    COALESCE(...) IS NOT NULL no-op behavior).
--   * IS DISTINCT FROM            — skip rows that already match, so this is a no-op re-run.
--
-- Purely additive UPDATEs against existing rows — no INSERT, so this cannot duplicate mst_mb_spin
-- rows or create a new master product, matching the hard constraint the rest of this fix follows.
--
-- BEGIN/COMMIT explicit, matching house style (000486/000490/000495/000496/000506).

BEGIN;

-- (a) mst_mb_spin root row.
UPDATE mst_mb_spin s
SET mbs_shade_code = h.mbh_shade_code,
    mbs_shade_name = h.mbh_shade_name,
    mbs_cross_section = h.mbh_cross_section,
    mbs_cc = h.mbh_shade_code,
    updated_at = NOW(),
    updated_by = 'migration_000507_backfill'
FROM mst_mb_head h
WHERE s.mbs_mbh_id = h.mbh_id
  AND s.mbs_parent_spin_id IS NULL
  AND s.deleted_at IS NULL
  AND (h.mbh_shade_code IS NOT NULL OR h.mbh_shade_name IS NOT NULL OR h.mbh_cross_section IS NOT NULL)
  AND (
        s.mbs_shade_code IS DISTINCT FROM h.mbh_shade_code
     OR s.mbs_shade_name IS DISTINCT FROM h.mbh_shade_name
     OR s.mbs_cross_section IS DISTINCT FROM h.mbh_cross_section
     OR s.mbs_cc IS DISTINCT FROM h.mbh_shade_code
  );

-- (b) linked cost_product_master row (Master Product MB), only for MB-recipe-sourced products.
UPDATE cost_product_master cpm
SET cpm_shade_code = h.mbh_shade_code,
    cpm_shade_name = h.mbh_shade_name,
    cpm_updated_at = NOW(),
    cpm_updated_by = 'migration_000507_backfill'
FROM mst_mb_head h
WHERE h.mbh_cost_product_id = cpm.cpm_product_sys_id
  AND cpm.cpm_source = 'MB_RECIPE'
  AND (h.mbh_shade_code IS NOT NULL OR h.mbh_shade_name IS NOT NULL)
  AND (
        cpm.cpm_shade_code IS DISTINCT FROM h.mbh_shade_code
     OR cpm.cpm_shade_name IS DISTINCT FROM h.mbh_shade_name
  );

COMMIT;
