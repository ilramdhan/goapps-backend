-- 000506: One-time backfill of the root MB Spin's (mbs_parent_spin_id IS NULL) computed LDR from
-- its parent MB Head, closing the gap the 31 Aug production bug left behind.
--
-- Background: before this fix, the root spin's mbs_ldr_calculated_pct was only ever seeded once,
-- at the very first Validate of a recipe (mbBuildAutoGenSpin / mst_mb_spin insert, migration
-- 000496). Every later re-validate/regenerate went through regenerateCostProductRMs
-- (mb_autogen_repository.go), which only ever touched cost_route_rm and mst_mb_head — it never
-- wrote back to mst_mb_spin. Recipes that were edited and re-validated one or more times after
-- their first validate were left showing a stale mbs_ldr_calculated_pct on the MB Recipe page,
-- out of sync with mst_mb_head.mbh_run_ldr_pct / mbh_ldr_prsn. The application-code half of this
-- fix (syncRootSpinLDRFromHead, mb_autogen_repository.go) makes every future re-validate keep the
-- two in sync going forward; this migration catches up every row that already drifted before the
-- fix shipped.
--
-- Column names verified directly against the migrations that created them:
--   * mst_mb_head:  mbh_id, mbh_run_ldr_pct, mbh_ldr_prsn        (000369/000414/000418 lineage)
--   * mst_mb_spin:  mbs_mbh_id, mbs_parent_spin_id, mbs_ldr_is_actual,
--                    mbs_ldr_calculated_pct, mbs_ldr_type          (000389/000484/000496)
--
-- Same precedence rule as the application code (Decision D3, 31 Aug): prefer the head's
-- already-recalculated run value (mbh_run_ldr_pct), falling back to its persisted/frozen value
-- (mbh_ldr_prsn) when no run value exists.
--
-- Guards, mirroring syncRootSpinLDRFromHead exactly:
--   * mbs_parent_spin_id IS NULL      — root spins only, never a Duplicate MB Spin clone.
--   * mbs_ldr_is_actual = FALSE       — never overwrite a human-locked ACTUAL value.
--   * deleted_at IS NULL              — never touch a soft-deleted spin.
--   * COALESCE(...) IS NOT NULL       — never write NULL over an existing value (recipes with no
--                                        LDR at all are left untouched, same as the app-code no-op).
--   * IS DISTINCT FROM                — skip rows that already match, so this is a no-op re-run.
--
-- Purely additive UPDATE against existing rows — no INSERT, so it cannot duplicate mst_mb_spin or
-- create a new master product, matching the hard constraint the rest of this fix follows.
--
-- BEGIN/COMMIT explicit, matching house style (000486/000490/000495/000496).

BEGIN;

UPDATE mst_mb_spin s
SET mbs_ldr_calculated_pct = COALESCE(h.mbh_run_ldr_pct, h.mbh_ldr_prsn),
    mbs_ldr_type = 'CALCULATED',
    updated_at = NOW(),
    updated_by = 'migration_000506_backfill'
FROM mst_mb_head h
WHERE s.mbs_mbh_id = h.mbh_id
  AND s.mbs_parent_spin_id IS NULL
  AND s.mbs_ldr_is_actual = FALSE
  AND s.deleted_at IS NULL
  AND COALESCE(h.mbh_run_ldr_pct, h.mbh_ldr_prsn) IS NOT NULL
  AND s.mbs_ldr_calculated_pct IS DISTINCT FROM COALESCE(h.mbh_run_ldr_pct, h.mbh_ldr_prsn);

COMMIT;
