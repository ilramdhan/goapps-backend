-- 000523 (oil-cost-rm-group M4) — convert the oil parameters.
--
--   OIL_NAME             CALCULATED -> MASTER_LOOKUP, lookup_master_code = 'RM_GROUP_OIL'
--                        (000522). Stored value = RM group code in cpp_value_text (D3).
--   OIL_RATE             stays RATE/NUMBER/USD; becomes a fill-group child of OIL_NAME
--                        (lookup_fill_group_code = 'OIL_NAME', lookup_source_column NULL)
--                        so it is auto-added to CAPP with OIL_NAME and renders read-only.
--                        The calc engine resolves it per period from the OIL_NAME group's
--                        cst_rm_cost row (CR -> SR -> PR); stored values are ignored, kept (D5/D14).
--   OIL_GAIN_POY_DEFAULT NEW. NUMBER, CALCULATED, USD, display group Analysis, fill-group
--                        child of OIL_NAME. Produced by the CONSTANT formula
--                        F_YARN_OIL_GAIN_POY_DEFAULT (000524), editable in Master Formula (D6).
--   OIL_GAIN             uom -> USD (guarded; already USD per 000407).
--
-- EXACT REVERSIBILITY: the pre-change values of every column this migration
-- touches on OIL_NAME / OIL_RATE / OIL_GAIN are copied into
-- bak_oil_params_000523 BEFORE the updates, and the .down.sql restores them
-- verbatim from there. No reliance on remembering what 000407/000392 seeded.
--
-- Idempotent: the backup INSERT is ON CONFLICT DO NOTHING (first run's values
-- win), updates are value-guarded, the new param INSERT is NOT EXISTS-guarded.

BEGIN;

-- ============================================================
-- PART 0: backup of the touched columns (for the down migration)
-- ============================================================
CREATE TABLE IF NOT EXISTS bak_oil_params_000523 (
    param_code             VARCHAR(50)  PRIMARY KEY,
    param_category         VARCHAR(20)  NOT NULL,
    lookup_master_code     VARCHAR(30),
    lookup_fill_group_code VARCHAR(50),
    lookup_source_column   VARCHAR(50),
    uom_id                 UUID,
    notes                  VARCHAR(500),
    updated_at             TIMESTAMPTZ,
    updated_by             VARCHAR(200),
    backed_up_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE bak_oil_params_000523 IS
    'Backup written by migration 000523 (oil-cost-rm-group M4); read by its down migration. Safe to drop once 000523 is final.';

INSERT INTO bak_oil_params_000523 (
    param_code, param_category, lookup_master_code, lookup_fill_group_code,
    lookup_source_column, uom_id, notes, updated_at, updated_by
)
SELECT p.param_code, p.param_category, p.lookup_master_code, p.lookup_fill_group_code,
       p.lookup_source_column, p.uom_id, p.notes, p.updated_at, p.updated_by
FROM mst_parameter p
WHERE p.param_code IN ('OIL_NAME', 'OIL_RATE', 'OIL_GAIN')
  AND p.deleted_at IS NULL
ON CONFLICT (param_code) DO NOTHING;

-- ============================================================
-- PART 1: OIL_NAME -> MASTER_LOOKUP over RM_GROUP_OIL
-- ============================================================
UPDATE mst_parameter
SET param_category     = 'MASTER_LOOKUP',
    lookup_master_code = 'RM_GROUP_OIL',
    notes              = 'Oil RM group code (RM_GROUP_OIL). Allowed/default groups come from the product type oil mapping.',
    updated_at         = NOW(),
    updated_by         = 'migration_000523'
WHERE param_code = 'OIL_NAME'
  AND deleted_at IS NULL
  AND (param_category IS DISTINCT FROM 'MASTER_LOOKUP'
       OR lookup_master_code IS DISTINCT FROM 'RM_GROUP_OIL');

-- ============================================================
-- PART 2: OIL_RATE -> fill-group child of OIL_NAME
-- ============================================================
UPDATE mst_parameter
SET lookup_fill_group_code = 'OIL_NAME',
    lookup_source_column   = NULL,
    notes                  = 'Resolved per period from OIL_NAME RM group (CR→SR→PR)',
    updated_at             = NOW(),
    updated_by             = 'migration_000523'
WHERE param_code = 'OIL_RATE'
  AND deleted_at IS NULL
  AND lookup_fill_group_code IS DISTINCT FROM 'OIL_NAME';

-- ============================================================
-- PART 3: new param OIL_GAIN_POY_DEFAULT
-- ============================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    lookup_fill_group_code, display_group, display_order, notes,
    is_active, created_at, created_by
)
SELECT
    'OIL_GAIN_POY_DEFAULT', 'Oil Gain POY Default', 'Oil Gain POY Default', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'OIL_NAME', 'Analysis', 151,
    'OIL_GAIN value for POY products; produced by CONSTANT formula F_YARN_OIL_GAIN_POY_DEFAULT (editable in Master Formula).',
    TRUE, NOW(), 'seed_000523'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'OIL_GAIN_POY_DEFAULT' AND deleted_at IS NULL
);

-- ============================================================
-- PART 4: OIL_GAIN uom -> USD (guarded: no-op when already USD or USD missing)
-- ============================================================
UPDATE mst_parameter p
SET uom_id     = u.uom_id,
    updated_at = NOW(),
    updated_by = 'migration_000523'
FROM mst_uom u
WHERE p.param_code = 'OIL_GAIN'
  AND p.deleted_at IS NULL
  AND u.uom_code = 'USD'
  AND u.deleted_at IS NULL
  AND p.uom_id IS DISTINCT FROM u.uom_id;

DO $$
DECLARE
    v_name TEXT;
    v_rate TEXT;
    v_new  INT;
BEGIN
    SELECT param_category || '/' || COALESCE(lookup_master_code, '-') INTO v_name
    FROM mst_parameter WHERE param_code = 'OIL_NAME' AND deleted_at IS NULL;
    SELECT COALESCE(lookup_fill_group_code, '-') INTO v_rate
    FROM mst_parameter WHERE param_code = 'OIL_RATE' AND deleted_at IS NULL;
    SELECT COUNT(*) INTO v_new
    FROM mst_parameter WHERE param_code = 'OIL_GAIN_POY_DEFAULT' AND deleted_at IS NULL;
    RAISE NOTICE '000523: OIL_NAME=% ; OIL_RATE fill group=% ; OIL_GAIN_POY_DEFAULT present=%',
        COALESCE(v_name, '(missing)'), COALESCE(v_rate, '(missing)'), v_new;
END $$;

COMMIT;
