-- 000508: Backfill mst_mb_head.mbh_shade_code / mbh_shade_name for LEGACY recipes (Oracle-imported
-- via 000417), then re-propagate to mst_mb_spin / cost_product_master — closing the root cause
-- behind 000507's observed no-op in production: 000507's sync only propagates FROM
-- mst_mb_head.mbh_shade_code/mbh_shade_name, but those columns were NEVER populated for rows
-- imported from Oracle CSV (000417) — only mbh_code (Oracle CMBH_CODE) carries the legacy shade
-- code, and mbh_shade_name has NO legacy counterpart at all (must be resolved from the Master
-- Shade table, cost_erp_shade, added later by 000105/000493/a4decba).
--
-- Column names verified against the migrations that created them:
--   * mst_mb_head:    mbh_id, mbh_code (000417, legacy Oracle CMBH_CODE), mbh_shade_code,
--                       mbh_shade_name (000445, new Master Shade fields)
--   * cost_erp_shade: ces_shade_code, ces_shade_name — natural key ces_shade_code, UNIQUE
--                       (uk_cost_erp_shade_code, 000105) so the JOIN below can never fan out to
--                       more than one row per mbh_code.
--
-- Order matters: Step A must run before Step B so a row's mbh_shade_code freshly filled by Step A
-- is immediately eligible for Step B's name lookup in the same migration. Step C (mirroring 000507
-- verbatim) must run last so mst_mb_spin/cost_product_master pick up both.
--
-- Guards:
--   * mbh_code IS NOT NULL AND mbh_code <> ''  — never backfill from a blank legacy code.
--   * mbh_shade_code IS NULL                    — never overwrite a value the new Master Shade
--                                                   flow (or the SHADE-01-style manual case) already
--                                                   set; purely additive for genuinely-empty rows.
--   * UPPER(TRIM(...))                          — normalizes case/whitespace, matching the existing
--                                                   convention in shade_repository.go (LOWER(...)/
--                                                   UPPER(TRIM(...)) lookups against cost_erp_shade).
--   * IS DISTINCT FROM                          — idempotent, safe to re-run.
--
-- Purely additive UPDATEs — no INSERT/DELETE, cannot duplicate any row or create a new master
-- product, same hard constraint the rest of the 31 Aug shade fix follows.
--
-- BEGIN/COMMIT explicit, matching house style (000486/000490/000495/000496/000506/000507).

BEGIN;

-- Step A: backfill mbh_shade_code from the legacy mbh_code for rows that never got a Master Shade
-- value. Deliberately does NOT normalize the value being WRITTEN (mbh_code is copied verbatim) —
-- only the JOIN condition in Step B is normalized, so the stored code stays visually identical to
-- what Oracle originally exported.
UPDATE mst_mb_head
SET mbh_shade_code = mbh_code,
    updated_at = NOW(),
    updated_by = 'migration_000508_backfill'
WHERE mbh_shade_code IS NULL
  AND mbh_code IS NOT NULL
  AND mbh_code <> ''
  AND deleted_at IS NULL;

-- Step B: resolve mbh_shade_name from cost_erp_shade using the (now backfilled) mbh_shade_code.
-- UPPER(TRIM(...)) on both sides guards against case/whitespace drift between the legacy Oracle
-- code and the Master Shade master, mirroring the LOWER(...)/UPPER(TRIM(...)) normalization already
-- used by shade_repository.go's lookups. Rows whose code has no match in cost_erp_shade are left
-- with mbh_shade_name = NULL (not an error) — a genuinely retired/legacy-only shade code.
UPDATE mst_mb_head h
SET mbh_shade_name = ces.ces_shade_name,
    updated_at = NOW(),
    updated_by = 'migration_000508_backfill'
FROM cost_erp_shade ces
WHERE h.deleted_at IS NULL
  AND h.mbh_shade_code IS NOT NULL
  AND h.mbh_shade_name IS NULL
  AND UPPER(TRIM(ces.ces_shade_code)) = UPPER(TRIM(h.mbh_shade_code))
  AND ces.ces_shade_name IS NOT NULL
  AND h.mbh_shade_name IS DISTINCT FROM ces.ces_shade_name;

-- Step C: re-run 000507's propagation verbatim so mst_mb_spin / cost_product_master pick up the
-- mbh_shade_code/mbh_shade_name values Step A/B just filled in. Identical logic/guards to 000507 —
-- intentionally duplicated here (not refactored into a shared function) so this migration stays a
-- standalone, replayable unit and 000507 remains untouched, per house rule against editing shipped
-- migrations on this branch.

-- (c1) mst_mb_spin root row.
UPDATE mst_mb_spin s
SET mbs_shade_code = h.mbh_shade_code,
    mbs_shade_name = h.mbh_shade_name,
    mbs_cross_section = h.mbh_cross_section,
    mbs_cc = h.mbh_shade_code,
    updated_at = NOW(),
    updated_by = 'migration_000508_backfill'
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

-- (c2) linked cost_product_master row (Master Product MB), only for MB-recipe-sourced products.
UPDATE cost_product_master cpm
SET cpm_shade_code = h.mbh_shade_code,
    cpm_shade_name = h.mbh_shade_name,
    cpm_updated_at = NOW(),
    cpm_updated_by = 'migration_000508_backfill'
FROM mst_mb_head h
WHERE h.mbh_cost_product_id = cpm.cpm_product_sys_id
  AND cpm.cpm_source = 'MB_RECIPE'
  AND (h.mbh_shade_code IS NOT NULL OR h.mbh_shade_name IS NOT NULL)
  AND (
        cpm.cpm_shade_code IS DISTINCT FROM h.mbh_shade_code
     OR cpm.cpm_shade_name IS DISTINCT FROM h.mbh_shade_name
  );

COMMIT;
