-- 000496: MB Spin auto-computed LDR tracking + shade/cross-section copy-down
-- from MB Head.
--
-- Business decision (already made by the business stakeholder, not this
-- migration): every mst_mb_spin row will get an automatically-computed LDR
-- (dozing) value going forward. This migration adds the columns that
-- decision needs. No Go application code is touched by this file (task
-- scope: migrations only).
--
-- ⛔ NAMING SAFETY — do not use bare 'RND' anywhere below. mst_mb_spin
-- already has an UNRELATED column mbs_lesture (Oracle passthrough,
-- 000414/000415) whose value can literally be the text 'RND' (a thread-type
-- code, see mst_mb_cross_section seed 000479:57 which ALSO happens to seed a
-- cross-section row coded 'RND' — two more pre-existing, unrelated uses of
-- the string). The new LDR-type states are spelled out in full instead:
-- NOT_CALCULATED / CALCULATED / ACTUAL. None of them is 'RND'.
--
-- LDR-type tri-state column, mbs_ldr_type:
--   * NOT_CALCULATED — default. New/auto-generated rows start here.
--   * CALCULATED     — value was produced by the recalculation formula.
--   * ACTUAL         — manually confirmed/locked by a human; the future
--                       recalculation cascade must skip rows in this state.
--   VARCHAR(20) + CHECK, NOT a Postgres ENUM type: this repo has zero
--   precedent for CREATE TYPE ... AS ENUM anywhere under migrations/postgres
--   (checked before writing this file) — every existing tri/multi-state
--   column here (e.g. job_execution.status, mst_uom.uom_category,
--   cst_rm_group_head.flag_valuation) uses VARCHAR + a named CHECK
--   constraint instead, so mbs_ldr_type follows that same house style.
--
-- ⛔ SEPARATE FROM mbs_ldr_is_fixed — this migration deliberately does NOT
-- reuse or merge with mbs_ldr_is_fixed (000486). That existing boolean is a
-- distinct, adjacent concept (NULL/FALSE/TRUE tri-semantics for "does the
-- recalc cascade skip this value", introduced for the P12b/P13 recalc
-- rules). The new mbs_ldr_is_actual boolean below is a brand new column,
-- NOT NULL DEFAULT FALSE (plain two-state, no NULL semantics), purely for
-- "has a human locked this LDR value as ACTUAL".
--
-- Calculated/adjustment numeric pair, mirroring the existing mbs_ldr_prsn /
-- mbs_run_ldr_pct precedent (both NUMERIC(10,4), 000418/000414):
--   * mbs_ldr_calculated_pct — the LDR value the formula computed.
--   * mbs_ldr_adjustment_pct — a manual override/adjustment amount applied
--                               on top of the calculated value.
--
-- Shade/cross-section copy-down columns — these do NOT exist on
-- mst_mb_spin at all today. They mirror mst_mb_head's equivalent columns
-- (mbh_shade_code VARCHAR(20), mbh_shade_name VARCHAR(100),
-- mbh_cross_section VARCHAR(20), all added by 000445) exactly in type and
-- length, with the mbs_ prefix used throughout this table. They are filled
-- by copying from the parent MB Head at MB Spin auto-generation time (that
-- copy logic is application code, out of scope for this migration).
--
-- All columns are additive, nullable-or-defaulted, and do not rewrite any
-- existing row's data-bearing content: mbs_ldr_type gets a DEFAULT so the
-- NOT NULL constraint is satisfiable for existing rows without a separate
-- UPDATE step, and mbs_ldr_is_actual is a plain NOT NULL DEFAULT FALSE for
-- the same reason. This mirrors the "column-only, no backfill UPDATE
-- needed" shape used by 000486 for a boolean-with-default add.
--
-- BEGIN/COMMIT explicit, matching house style (000486/000490/000495).

BEGIN;

ALTER TABLE mst_mb_spin
    ADD COLUMN IF NOT EXISTS mbs_shade_code         VARCHAR(20),
    ADD COLUMN IF NOT EXISTS mbs_shade_name         VARCHAR(100),
    ADD COLUMN IF NOT EXISTS mbs_cross_section      VARCHAR(20),
    ADD COLUMN IF NOT EXISTS mbs_ldr_type           VARCHAR(20) NOT NULL DEFAULT 'NOT_CALCULATED',
    ADD COLUMN IF NOT EXISTS mbs_ldr_calculated_pct NUMERIC(10,4),
    ADD COLUMN IF NOT EXISTS mbs_ldr_adjustment_pct NUMERIC(10,4),
    ADD COLUMN IF NOT EXISTS mbs_ldr_is_actual       BOOLEAN     NOT NULL DEFAULT FALSE;

-- Idempotent CHECK add: ADD CONSTRAINT has no IF NOT EXISTS in PostgreSQL,
-- so guard with a DO-block, mirroring 000490's fk_mbs_cost_product pattern.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_mbs_ldr_type'
  ) THEN
    ALTER TABLE mst_mb_spin
      ADD CONSTRAINT chk_mbs_ldr_type
        CHECK (mbs_ldr_type IN ('NOT_CALCULATED', 'CALCULATED', 'ACTUAL'));
  END IF;
END
$$;

COMMENT ON COLUMN mst_mb_spin.mbs_shade_code IS
  'Shade code copied from parent mst_mb_head.mbh_shade_code at MB Spin auto-generation time.';
COMMENT ON COLUMN mst_mb_spin.mbs_shade_name IS
  'Shade name copied from parent mst_mb_head.mbh_shade_name at MB Spin auto-generation time.';
COMMENT ON COLUMN mst_mb_spin.mbs_cross_section IS
  'Cross section copied from parent mst_mb_head.mbh_cross_section at MB Spin auto-generation time.';
COMMENT ON COLUMN mst_mb_spin.mbs_ldr_type IS
  'LDR value provenance/state: NOT_CALCULATED (default, new/auto-generated rows) | '
  'CALCULATED (produced by the recalculation formula) | ACTUAL (manually confirmed/locked '
  'by a human — the recalculation cascade must not overwrite rows in this state). '
  'Deliberately spelled out in full rather than abbreviated to avoid colliding with the '
  'unrelated mbs_lesture Oracle thread-type code, which can literally be the text ''RND''.';
COMMENT ON COLUMN mst_mb_spin.mbs_ldr_calculated_pct IS
  'LDR value produced by the recalculation formula. Separate from mbs_ldr_adjustment_pct, '
  'which holds a manual override applied on top of this value.';
COMMENT ON COLUMN mst_mb_spin.mbs_ldr_adjustment_pct IS
  'Manual override/adjustment amount a user applies on top of mbs_ldr_calculated_pct.';
COMMENT ON COLUMN mst_mb_spin.mbs_ldr_is_actual IS
  'TRUE = a human has locked this LDR value as ACTUAL (never overwritten by recalculation). '
  'Brand new flag, deliberately separate from the pre-existing mbs_ldr_is_fixed (000486), '
  'which is a distinct, adjacent recalc-skip concept and must not be merged with this one.';

COMMIT;
