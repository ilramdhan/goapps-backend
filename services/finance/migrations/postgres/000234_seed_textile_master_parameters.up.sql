-- 000234: Textile master parameters catalog (S8e-fix 1/3).
--
-- Seeds master parameters that exercise the calc engine on a realistic
-- Indonesian polyester / textile mill cost structure. Reuses existing canonical
-- parameters where they cover the same semantics (WASTE_PCT, LABOR_HRS,
-- LABOR_RATE, ELEC_KWH, ELEC_RATE, LABOR_OVERHEAD_PCT, MAT_OVERHEAD_PCT,
-- DEPREC_PER_KG, plus the existing chained calculated sinks COST_RM_TOTAL,
-- COST_RM_LOADED, COST_ELEC, COST_LABOR, COST_LABOR_FULL, COST_CONVERSION,
-- COST_STAGE_OUT). Only ADDS new params for textile realism.
--
-- NOTE on param_code naming: mst_parameter.param_code is VARCHAR(20). All new
-- codes are kept <=20 chars. The verbose names from the S8e-fix brief
-- (MACHINE_DEPRECIATION_PER_KG, ELECTRICITY_RATE_PER_KWH, etc.) are
-- abbreviated.
--
-- Idempotency: per-row INSERT...SELECT...WHERE NOT EXISTS guard. The unique
-- index on param_code is partial (deleted_at IS NULL) so ON CONFLICT cannot
-- be used directly.

BEGIN;

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, v.data_type, v.param_category,
       v.default_value, TRUE, 'seed_000234', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    -- New INPUT params (process losses + yields)
    ('YIELD_PCT',       'Process yield percent',              'Yield %',         'NUMBER', 'INPUT', 96.5::NUMERIC,  'Production',  'CONVERSION', 10),
    ('SHRINK_PCT',      'Heat-set shrinkage percent',         'Shrink %',        'NUMBER', 'INPUT',  4.0::NUMERIC,  'Production',  'CONVERSION', 11),
    ('OIL_PICKUP_PCT',  'Spin finish oil pickup percent',     'Oil Pickup %',    'NUMBER', 'INPUT',  0.8::NUMERIC,  'Production',  'CONVERSION', 12),
    ('IND_LABOR_PCT',   'Indirect labor as % of direct',      'Indir Labor %',   'NUMBER', 'INPUT', 25.0::NUMERIC,  'Finance',     'CONVERSION', 13),
    -- New INPUT params (machine + utilities)
    ('MACHINE_PER_KG',  'Machine running cost per kg',        'Machine /kg',     'NUMBER', 'INPUT',    0::NUMERIC,  'Engineering', 'CONVERSION', 20),
    ('STEAM_KG',        'Steam consumption (kg steam / kg)',  'Steam kg/kg',     'NUMBER', 'INPUT',    0::NUMERIC,  'Engineering', 'CONVERSION', 21),
    ('STEAM_RATE',      'Steam rate (IDR / kg)',              'Steam Rate',      'NUMBER', 'RATE',   800::NUMERIC,  'Finance',     'RATES',      21),
    ('WATER_M3',        'Water consumption (m3 / kg)',        'Water m3/kg',     'NUMBER', 'INPUT',    0::NUMERIC,  'Engineering', 'CONVERSION', 22),
    ('WATER_RATE',      'Water rate (IDR / m3)',              'Water Rate',      'NUMBER', 'RATE', 12000::NUMERIC,  'Finance',     'RATES',      22),
    ('AIR_PER_KG',      'Compressed air cost (IDR / kg)',     'Air /kg',         'NUMBER', 'INPUT',    0::NUMERIC,  'Engineering', 'CONVERSION', 23),
    -- New INPUT params (maintenance + overhead)
    ('MAINT_PER_KG',    'Maintenance cost per kg (IDR / kg)', 'Maint /kg',       'NUMBER', 'INPUT',    0::NUMERIC,  'Engineering', 'OVERHEAD', 30),
    ('FACTORY_OH',      'Factory overhead (IDR / kg)',        'Factory OH /kg',  'NUMBER', 'INPUT',    0::NUMERIC,  'Finance',     'OVERHEAD', 31),
    ('QC_PER_KG',       'QC cost per kg (IDR / kg)',          'QC /kg',          'NUMBER', 'INPUT',    0::NUMERIC,  'Production',  'OVERHEAD', 32),
    ('PACK_PER_KG',     'Packing cost per kg (IDR / kg)',     'Packing /kg',     'NUMBER', 'INPUT',    0::NUMERIC,  'Production',  'OVERHEAD', 33),
    ('MARGIN_PCT',      'Gross margin percent (for SELLING)', 'Margin %',        'NUMBER', 'INPUT',   18::NUMERIC,  'Finance',     'OVERHEAD', 40)
  ) AS v(param_code, param_name, param_short_name, data_type, param_category, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p
        WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- New CALCULATED params (additional sinks).
-- NULL default_value because data_type=NUMBER computed params have no entered default.
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    is_active, created_by, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'CALCULATED',
       TRUE, 'seed_000234', v.display_group, v.display_order
  FROM (VALUES
    ('COST_STEAM',      'Calculated steam cost (IDR / kg)',   'Cost Steam',      'COST_OUTPUT', 11),
    ('COST_WATER',      'Calculated water cost (IDR / kg)',   'Cost Water',      'COST_OUTPUT', 12),
    ('COST_UTIL',       'Utility cost total (IDR / kg)',      'Cost Utility',    'COST_OUTPUT', 13),
    ('COST_OVERHEAD',   'Total overhead cost (IDR / kg)',     'Cost Overhead',   'COST_OUTPUT', 14),
    ('COST_AFTER_YLD',  'Cost after yield correction',        'Cost After Yld',  'COST_OUTPUT', 15),
    ('SELLING_PRICE',   'Selling price (IDR / kg)',           'Selling',         'COST_OUTPUT', 20)
  ) AS v(param_code, param_name, param_short_name, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p
        WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

COMMIT;
