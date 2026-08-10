-- 000474: Spin fixed-cost pool master (POY per-kg fixed cost model).
--
-- WHY: Legacy PkgFormulaYarn.fPoyPower_87/88/89/90 compute POY POWER/MANPOWER/
-- OVERHEAD/SPARES per-kg from a SHARED monthly cost pool, NOT from per-machine
-- POWER_PER_DAY / NET_PRODUCTION. Source = MGTAPPS.MST_PARAM_DATA group
-- 'MST_SPIN_FIXED_COST_4_AVG' (Finance-maintained, current-only). The goapps
-- engine had no equivalent, so per-kg fixed costs were computed with the wrong
-- (bottom-up) model and inflated ~8.4x.
--
-- Legacy formula reproduced:
--   *_PER_KG = spin_xxx_month / poy_production * common_poy_denier
--                             / ACT_DENIER * mc_weightage
--
-- VERIFIED against product cpm_product_sys_id 90299 (cpm_flex_02 = '83',
-- 'POY 250/48/RND/SD/SIM/NS/1/O'), machine C-10-S (mc_weightage 1.0000),
-- ACT_DENIER 250:
--   198634 / 3027153 * 329.712 / 250 * 1.0 = 0.0865392   (legacy CYCC TOP 87 = 0.0865)
--
-- INPUT data (Finance-maintained), NOT calculated — safe to seed. Values are the
-- CURRENT MST_PARAM_DATA values, tagged as period 202607 (legacy is current-only).
-- Period-scoped so future periods add rows instead of overwriting history.

BEGIN;

CREATE TABLE IF NOT EXISTS mst_spin_fixed_cost (
  msfc_id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  msfc_period                VARCHAR(6)    NOT NULL CHECK (msfc_period ~ '^[0-9]{6}$'),
  msfc_common_poy_denier     NUMERIC(20,6) NOT NULL,  -- legacy MPD 20210900147 "Common POY Denier"
  msfc_poy_production        NUMERIC(20,6) NOT NULL,  -- legacy MPD 20210900149 "POY production"
  msfc_spin_power_month      NUMERIC(20,6) NOT NULL,  -- legacy MPD 20210900151 "Spinning power / month"
  msfc_spin_manpower_month   NUMERIC(20,6) NOT NULL,  -- legacy MPD 20210900152 "Spinning manpower / month"
  msfc_spin_overheads_month  NUMERIC(20,6) NOT NULL,  -- legacy MPD 20210900153 "Spinning Overheads / month"
  msfc_spin_conssprs_month   NUMERIC(20,6) NOT NULL,  -- legacy MPD 20210900154 "Spinning Cons,Spares / month"
  msfc_is_active             BOOLEAN       NOT NULL DEFAULT TRUE,
  msfc_created_at            TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
  msfc_created_by            VARCHAR(100)  NOT NULL,
  msfc_updated_at            TIMESTAMPTZ,
  msfc_updated_by            VARCHAR(100),
  deleted_at                 TIMESTAMPTZ,
  deleted_by                 VARCHAR(100)
);

-- Partial unique: one LIVE row per period. A soft-deleted row must not block a
-- replacement (the plain UNIQUE in the original draft contradicted its own
-- deleted_at-aware seed guard).
CREATE UNIQUE INDEX IF NOT EXISTS uq_msfc_period_live
  ON mst_spin_fixed_cost (msfc_period) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_spin_fixed_cost IS
  'POY spin fixed-cost pool per period. Feeds POY *_PER_KG fixed cost formulas via '
  'engine scope injection. Legacy source: MST_PARAM_DATA group MST_SPIN_FIXED_COST_4_AVG.';

-- Anchor row, seeded at the earliest period the engine has to cost (202604).
--
-- The values are the current MST_PARAM_DATA snapshot. Legacy stores them
-- current-only with no history, so legacy recomputing 202604 and 202607 both
-- read this same set — one anchor row reproduces that faithfully. The loader
-- resolves the newest row at or before the requested period, so this covers
-- every period from 202604 onward until finance enters a real monthly row,
-- which then takes over from its own period forward with no code change.
--
-- Anchoring at the earliest period rather than the latest also keeps local,
-- staging and production identical: they hold different period ranges, and a
-- 202607 anchor would resolve to nothing on a local database that only has
-- 202604 data.
INSERT INTO mst_spin_fixed_cost (
  msfc_period, msfc_common_poy_denier, msfc_poy_production,
  msfc_spin_power_month, msfc_spin_manpower_month,
  msfc_spin_overheads_month, msfc_spin_conssprs_month, msfc_created_by)
SELECT '202604', 329.712, 3027153, 198634, 275561, 46600, 54100, 'seed_spin_pool_000474'
WHERE NOT EXISTS (
  SELECT 1 FROM mst_spin_fixed_cost WHERE msfc_period = '202604' AND deleted_at IS NULL
);

COMMIT;
