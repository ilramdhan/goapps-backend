-- 000480: MB cross-section conversion factors + the 3 LDR formula rows.
--
-- SHAPE = DIRECTED PAIR TABLE (from_code, to_code, factor, operation).
-- This is user decision K-9 (2026-08-22) and design §2.8. It is deliberately
-- NOT the "index E per product" shape proposed in the LDR Calculator PRD:
-- the conversion is a property of the ORDERED PAIR of cross sections, and the
-- operation differs per direction (RND→TBL divides, TBL→RND multiplies). An
-- index-per-product column cannot express the direction, forcing the Go code to
-- guess which way to apply the factor. Do not "simplify" this back into a single
-- factor column.
--
-- Design reference numbered this migration 000483; the applied number is 000480
-- because the intervening design migrations are not part of this batch.
--
-- This migration also carries two mechanical consequences of user decision K-10
-- (see PART 2 and PART 3): the 3 formula rows live here, so the virtual result
-- params they point at and the formula_type CHECK widening must live here too.

BEGIN;

-- ============================================================
-- PART 1: The directed-pair factor table
-- ============================================================

CREATE TABLE IF NOT EXISTS mst_mb_cross_section_factor (
  mbcf_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbcf_from_code     VARCHAR(10)    NOT NULL,
  mbcf_to_code       VARCHAR(10)    NOT NULL,
  mbcf_factor        NUMERIC(12,6)  NOT NULL CHECK (mbcf_factor > 0),
  mbcf_operation     VARCHAR(10)    NOT NULL DEFAULT 'MULTIPLY'
                     CHECK (mbcf_operation IN ('MULTIPLY','DIVIDE')),
  mbcf_note          VARCHAR(200),
  mbcf_is_active     BOOLEAN        NOT NULL DEFAULT TRUE,
  mbcf_created_at    TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
  mbcf_created_by    VARCHAR(20)    NOT NULL,
  mbcf_updated_at    TIMESTAMPTZ,
  mbcf_updated_by    VARCHAR(20),
  deleted_at         TIMESTAMPTZ,
  deleted_by         VARCHAR(20),
  CONSTRAINT chk_mbcf_not_self CHECK (mbcf_from_code <> mbcf_to_code),
  CONSTRAINT fk_mbcf_from FOREIGN KEY (mbcf_from_code)
    REFERENCES mst_mb_cross_section (mbcs_code) ON UPDATE CASCADE,
  CONSTRAINT fk_mbcf_to FOREIGN KEY (mbcf_to_code)
    REFERENCES mst_mb_cross_section (mbcs_code) ON UPDATE CASCADE
);

-- Partial unique: one LIVE row per ordered pair. A soft-deleted row must not
-- block its replacement (same reasoning as uq_msfc_period_live in 000474).
CREATE UNIQUE INDEX IF NOT EXISTS uix_mbcf_pair
  ON mst_mb_cross_section_factor (mbcf_from_code, mbcf_to_code) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_mb_cross_section_factor IS
  'MB LDR cross-section conversion factors, one row per ORDERED pair. '
  'mbcf_operation carries the direction of the arithmetic; it is not derivable '
  'from the factor alone.';

-- Seed v1 = exactly 2 rows. Idempotent, guarded per row.
INSERT INTO mst_mb_cross_section_factor
  (mbcf_from_code, mbcf_to_code, mbcf_factor, mbcf_operation, mbcf_note, mbcf_created_by)
SELECT 'RND', 'TBL', 0.82, 'DIVIDE', 'LDR_target = LDR_source / 0.82 (design 2026-08-19)', 'seed_000480'
WHERE NOT EXISTS (
  SELECT 1 FROM mst_mb_cross_section_factor
  WHERE mbcf_from_code = 'RND' AND mbcf_to_code = 'TBL' AND deleted_at IS NULL
);

INSERT INTO mst_mb_cross_section_factor
  (mbcf_from_code, mbcf_to_code, mbcf_factor, mbcf_operation, mbcf_note, mbcf_created_by)
SELECT 'TBL', 'RND', 0.82, 'MULTIPLY', 'LDR_target = LDR_source * 0.82 (design 2026-08-19)', 'seed_000480'
WHERE NOT EXISTS (
  SELECT 1 FROM mst_mb_cross_section_factor
  WHERE mbcf_from_code = 'TBL' AND mbcf_to_code = 'RND' AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Virtual result parameters (user decision K-10, 2026-08-22)
-- ============================================================
-- mst_formula.result_param_id is NOT NULL and FKs to mst_parameter(id)
-- (000005_create_mst_formula.up.sql:8). The design assumed it was nullable; it
-- is not. K-10 resolves this by creating one dedicated result parameter per
-- formula rather than by ALTERing the column to nullable — the FK and the
-- NOT NULL stay untouched.
--
-- These 3 params are VIRTUAL: they exist only as the destination slot of a
-- lookup/preview formula. They are NOT input parameters — no user ever fills
-- them in a form, they are not wired into any fill group, and they carry no
-- CAPP/CPP rows. Their values flow back to the LDR calculator form as a preview.
--
-- Naming follows the repo's existing result-param convention: formula F_X writes
-- to param X (F_MB_WASTE_VAL → MB_WASTE_VAL, 000452). So F_MB_LDR_SCALE writes
-- to MB_LDR_SCALE. param_code is VARCHAR(20); all 3 codes fit.
-- param_category CALCULATED marks them as engine-produced, never user-entered.
-- display_group 'MasterBatch' with display_order in the free 70..72 band
-- (existing MasterBatch slots are 10..60).

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    display_group, display_order, is_active, created_at, created_by
)
SELECT p.code, p.name, p.short_name, 'NUMBER', 'CALCULATED',
       'MasterBatch', p.display_order, TRUE, NOW(), 'seed_000480'
FROM (VALUES
  ('MB_LDR_SCALE',    'MB LDR Scaled (virtual result)',            'MB LDR Scale',    70),
  ('MB_LDR_XSECTION', 'MB LDR Cross-Section Converted (virtual result)', 'MB LDR XSection', 71),
  ('MB_LDR_STRENGTH', 'MB LDR Strength Adjusted (virtual result)', 'MB LDR Strength', 72)
) AS p(code, name, short_name, display_order)
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 3: Widen formula_type CHECK — add MB_XSECTION_LOOKUP
-- ============================================================
-- Mechanical consequence of keeping the formula rows in this migration (K-10),
-- not a new decision. The live constraint holds 12 values, last extended by
-- 000451 (MB_COST_LOOKUP). F_MB_LDR_XSECTION needs a 13th, MB_XSECTION_LOOKUP:
-- like RM_LOOKUP / MB_COST_LOOKUP it requires dedicated Go code (a master-data
-- lookup into mst_mb_cross_section_factor), not plain expr-lang evaluation.
-- Same DROP + ADD pattern as 000451.

ALTER TABLE mst_formula DROP CONSTRAINT IF EXISTS mst_formula_formula_type_check;
ALTER TABLE mst_formula ADD CONSTRAINT mst_formula_formula_type_check
  CHECK (formula_type IN (
    'CALCULATION','SQL_QUERY','CONSTANT','LOOKUP','RM_LOOKUP','CONDITIONAL',
    'FROM_MARKETING','INTERMINGLING','SNAPSHOT','PENDING','INITIAL_VALUE',
    'MB_COST_LOOKUP','MB_XSECTION_LOOKUP'
  ));

-- ============================================================
-- PART 4: The 3 LDR formula rows (design §2.8, pattern of 000452 / 000469)
-- ============================================================
-- No formula_param rows are inserted: these 3 formulas are driven by the LDR
-- calculator's own scope (the *_REF / *_TARGET / XSECTION_* / STRENGTH inputs are
-- supplied by the calculator, not by cost-sheet params), so they take no part in
-- topoSortFormulas' dependency ordering. This mirrors F_MB_RM_COST in 000452,
-- which likewise has no formula_param rows.

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT f.code, f.name, f.ftype, f.expr,
       (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1),
       f.descr, 1, TRUE, NOW(), 'seed_000480'
FROM (VALUES
  ('F_MB_LDR_SCALE', 'MB LDR Scaling by Denier/Filament', 'CALCULATION',
   'LDR_REF * (sqrt(DENIER_REF / FILAMENT_REF) / sqrt(DENIER_TARGET / FILAMENT_TARGET))',
   'MB_LDR_SCALE',
   'LDR scaled from a reference recipe by denier/filament ratio (design 2026-08-19 §2.8)'),
  ('F_MB_LDR_XSECTION', 'MB LDR Cross Section Conversion', 'MB_XSECTION_LOOKUP',
   'XSECTION_OP == 1 ? LDR_SOURCE * XSECTION_FACTOR : LDR_SOURCE / XSECTION_FACTOR',
   'MB_LDR_XSECTION',
   'LDR converted across cross sections; factor and operation come from mst_mb_cross_section_factor via Go lookup code'),
  ('F_MB_LDR_STRENGTH', 'MB LDR Strength Adjustment', 'CALCULATION',
   'DIRECTION == 1 ? LDR_OLD - ((STRENGTH - 100) / 100) : LDR_OLD + ((STRENGTH - 100) / 100)',
   'MB_LDR_STRENGTH',
   'LDR adjusted for colorant strength difference (design 2026-08-19 §2.8)')
) AS f(code, name, ftype, expr, result_code, descr)
WHERE (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1) IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = f.code AND deleted_at IS NULL
  );

COMMIT;
