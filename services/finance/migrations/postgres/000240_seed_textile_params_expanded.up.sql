-- 000240: Expanded textile master parameters catalog (+80 params).
--
-- Brings active param catalog from 54 -> ~134, covering: polymer detail (12),
-- texturing detail (10), knitting/weaving (10), dyeing recipe (15),
-- finishing + QC (10), packing + logistics (8), extra utility rates (6),
-- and new CALCULATED sinks (9) for chemical + utility cost breakdowns.
--
-- Naming: mst_parameter.param_code is VARCHAR(20). All new codes <= 20 chars.
--   - TEX_DRAW_RATIO renamed from spec's DRAW_RATIO (existing active sink).
--
-- data_type CHECK = ('NUMBER','TEXT','BOOLEAN'). Use NUMBER for numerics.
-- param_category CHECK = ('INPUT','RATE','CALCULATED').
--
-- Idempotency: per-row INSERT ... SELECT ... WHERE NOT EXISTS guard.
-- The unique index on param_code is partial (deleted_at IS NULL), so
-- ON CONFLICT cannot be used directly.

BEGIN;

-- ============================================================================
-- INPUT params -- Polymer detail (12)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'INPUT',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('PTA_RATIO',       'PTA consumption ratio (kg/kg chip)',     'PTA Ratio',       0.86::NUMERIC, 'Production', 'POLYMER', 100),
    ('MEG_RATIO',       'MEG consumption ratio (kg/kg chip)',     'MEG Ratio',       0.34::NUMERIC, 'Production', 'POLYMER', 101),
    ('SPIN_OIL_RATIO',  'Spin finish oil ratio (kg/kg)',          'Spin Oil',        0.008::NUMERIC,'Production', 'POLYMER', 102),
    ('TIO2_RATIO',      'TiO2 additive ratio (kg/kg)',            'TiO2 Ratio',      0.003::NUMERIC,'Production', 'POLYMER', 103),
    ('POLY_VISCOSITY',  'Polymer melt viscosity (Pa.s)',          'Poly Viscosity',  280::NUMERIC,  'QC',         'POLYMER', 104),
    ('POLY_TIO2_PCT',   'Polymer TiO2 weight percent',            'TiO2 %',          0.3::NUMERIC,  'QC',         'POLYMER', 105),
    ('CHIP_DRY_HRS',    'Chip drying duration (hours)',           'Dry Hrs',         6::NUMERIC,    'Production', 'POLYMER', 106),
    ('CHIP_CRYST_TEMP', 'Chip crystallization temperature (C)',   'Cryst Temp',      170::NUMERIC,  'Production', 'POLYMER', 107),
    ('MELT_TEMP_C',     'Melt temperature (C)',                   'Melt Temp',       290::NUMERIC,  'Production', 'POLYMER', 108),
    ('SPINNERET_HOLES', 'Spinneret hole count',                   'Spin Holes',      72::NUMERIC,   'Production', 'POLYMER', 109),
    ('WIND_SPEED_MPM',  'Winder speed (m/min)',                   'Wind Speed',      3500::NUMERIC, 'Production', 'POLYMER', 110),
    ('THROUGHPUT_KG_HR','Spinning throughput (kg/hr)',            'Throughput',      45::NUMERIC,   'Production', 'POLYMER', 111)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- INPUT params -- Texturing detail (10) [TEX_DRAW_RATIO replaces DRAW_RATIO collision]
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'INPUT',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('TEX_DRAW_RATIO',  'Texturing draw ratio',                   'Tex Draw',        1.65::NUMERIC, 'Production', 'TEXTURING', 200),
    ('TEX_RATIO',       'Texturing speed ratio',                  'Tex Ratio',       1.02::NUMERIC, 'Production', 'TEXTURING', 201),
    ('TWIST_TPM',       'Twist (turns / meter)',                  'Twist TPM',       2500::NUMERIC, 'Production', 'TEXTURING', 202),
    ('TWIST_TPI',       'Twist (turns / inch)',                   'Twist TPI',       63::NUMERIC,   'Production', 'TEXTURING', 203),
    ('INTERMINGLE_NPM', 'Intermingle nips / meter',               'Intermingle',     120::NUMERIC,  'Production', 'TEXTURING', 204),
    ('HEATSET_TEMP_C',  'Heatsetter temperature (C)',             'Heatset Temp',    220::NUMERIC,  'Production', 'TEXTURING', 205),
    ('HEATSET_TIME_S',  'Heatsetter dwell time (sec)',            'Heatset Time',    1.2::NUMERIC,  'Production', 'TEXTURING', 206),
    ('PRESET_TENSION',  'Pre-set tension (cN)',                   'Preset Tension',  18::NUMERIC,   'Production', 'TEXTURING', 207),
    ('BREAK_STR_CN',    'Breaking strength (cN/tex)',             'Break Str',       35::NUMERIC,   'QC',         'TEXTURING', 208),
    ('ELONGATION_PCT',  'Elongation at break percent',            'Elongation %',    22::NUMERIC,   'QC',         'TEXTURING', 209)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- INPUT params -- Knitting / Weaving (10)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'INPUT',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('KNIT_GAUGE_E',    'Knit machine gauge (E)',                 'Gauge E',         28::NUMERIC,   'Production', 'KNITWEAVE', 300),
    ('KNIT_STITCH_LEN', 'Stitch length (mm)',                     'Stitch Len',      2.8::NUMERIC,  'Production', 'KNITWEAVE', 301),
    ('LOOPS_PER_INCH',  'Loops per inch (wales)',                 'Loops/in',        38::NUMERIC,   'Production', 'KNITWEAVE', 302),
    ('COURSES_PER_IN',  'Courses per inch',                       'Courses/in',      42::NUMERIC,   'Production', 'KNITWEAVE', 303),
    ('WEAVE_SETT_PCM',  'Weave ends per cm (sett)',               'Weave Sett',      28::NUMERIC,   'Production', 'KNITWEAVE', 304),
    ('WEAVE_PICKS_PCM', 'Weave picks per cm',                     'Weave Picks',     24::NUMERIC,   'Production', 'KNITWEAVE', 305),
    ('FABRIC_WIDTH_M',  'Fabric width (m)',                       'Fabric Width',    1.6::NUMERIC,  'Production', 'KNITWEAVE', 306),
    ('FABRIC_GSM',      'Fabric grammage (g/m2)',                 'GSM',             180::NUMERIC,  'Production', 'KNITWEAVE', 307),
    ('KNIT_NEEDLES',    'Knit needle count',                      'Needles',         2640::NUMERIC, 'Production', 'KNITWEAVE', 308),
    ('KNIT_DOFF_HRS',   'Knit doff cycle (hours)',                'Doff Hrs',        4::NUMERIC,    'Production', 'KNITWEAVE', 309)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- INPUT params -- Dyeing recipe (15)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'INPUT',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('DYESTUFF_OWF_PCT','Dyestuff on weight of fabric %',         'Dyestuff OWF',    2.5::NUMERIC,  'Dyeing',     'DYEING', 400),
    ('DYE_BATH_LR',     'Dye bath liquor ratio (1:X)',            'LR',              8::NUMERIC,    'Dyeing',     'DYEING', 401),
    ('LEVELING_AGT_GL', 'Leveling agent (g/L)',                   'Leveling',        1.2::NUMERIC,  'Dyeing',     'DYEING', 402),
    ('DISPERSING_GL',   'Dispersing agent (g/L)',                 'Dispersing',      1.0::NUMERIC,  'Dyeing',     'DYEING', 403),
    ('PH_VALUE',        'Bath pH value',                          'pH',              5.0::NUMERIC,  'Dyeing',     'DYEING', 404),
    ('PH_BUFFER_GL',    'pH buffer (g/L)',                        'Buffer',          0.5::NUMERIC,  'Dyeing',     'DYEING', 405),
    ('CAUSTIC_GL',      'Caustic soda (g/L)',                     'Caustic',         2.0::NUMERIC,  'Dyeing',     'DYEING', 406),
    ('SODA_ASH_GL',     'Soda ash (g/L)',                         'Soda Ash',        3.0::NUMERIC,  'Dyeing',     'DYEING', 407),
    ('ACETIC_ACID_GL',  'Acetic acid (g/L)',                      'Acetic',          0.5::NUMERIC,  'Dyeing',     'DYEING', 408),
    ('SOAPING_AGT_GL',  'Soaping agent (g/L)',                    'Soaping',         1.0::NUMERIC,  'Dyeing',     'DYEING', 409),
    ('DYE_TEMP_C',      'Dyeing temperature (C)',                 'Dye Temp',        130::NUMERIC,  'Dyeing',     'DYEING', 410),
    ('DYE_TIME_MIN',    'Dyeing cycle time (min)',                'Dye Time',        90::NUMERIC,   'Dyeing',     'DYEING', 411),
    ('DYE_BATHS_LOT',   'Dye baths per lot',                      'Baths/Lot',       1::NUMERIC,    'Dyeing',     'DYEING', 412),
    ('WATER_L_PER_KG',  'Water consumption (L/kg fabric)',        'Water L/kg',      80::NUMERIC,   'Dyeing',     'DYEING', 413),
    ('STEAM_KG_DYE',    'Steam consumption dyeing (kg/kg)',       'Steam kg/kg D',   3.5::NUMERIC,  'Dyeing',     'DYEING', 414)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- INPUT params -- Finishing + QC (10)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'INPUT',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('SOFTENER_OWF',    'Softener on weight of fabric %',         'Softener OWF',    2.0::NUMERIC,  'Finishing',  'FINISHING', 500),
    ('ANTISTAT_OWF',    'Antistatic on weight of fabric %',       'Antistat OWF',    0.5::NUMERIC,  'Finishing',  'FINISHING', 501),
    ('STENTER_SPD_MPM', 'Stenter line speed (m/min)',             'Stenter Spd',    25::NUMERIC,   'Finishing',  'FINISHING', 502),
    ('STENTER_TEMP_C',  'Stenter temperature (C)',                'Stenter Temp',  180::NUMERIC,   'Finishing',  'FINISHING', 503),
    ('INSPECT_REJ_PCT', 'Inspection rejection percent',           'Rej %',           2.5::NUMERIC,  'QC',         'FINISHING', 504),
    ('INSPECT_HR_TON',  'Inspection hours per ton',               'Insp Hr/Ton',    4::NUMERIC,    'QC',         'FINISHING', 505),
    ('CUT_LENGTH_M',    'Cut length per piece (m)',               'Cut Length',     50::NUMERIC,   'Finishing',  'FINISHING', 506),
    ('GSM_TOL_PCT',     'GSM tolerance percent',                  'GSM Tol %',       3.0::NUMERIC,  'QC',         'FINISHING', 507),
    ('COLOR_DEV_DE',    'Color deviation Delta E',                'dE',              0.8::NUMERIC,  'QC',         'FINISHING', 508),
    ('LAB_TEST_COST',   'Lab test cost (IDR / lot)',              'Lab /Lot',     150000::NUMERIC,  'QC',         'FINISHING', 509)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- INPUT params -- Packing + Logistics (8)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'INPUT',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('CONES_PER_KG',    'Cones per kg yarn',                      'Cones/kg',        0.4::NUMERIC,  'Packing',    'PACKLOG', 600),
    ('CARTON_PER_TON',  'Cartons per ton',                        'Cartons/Ton',    40::NUMERIC,   'Packing',    'PACKLOG', 601),
    ('STRETCH_FILM_PCT','Stretch film consumption %',             'Film %',          0.3::NUMERIC,  'Packing',    'PACKLOG', 602),
    ('PALLET_PER_TON',  'Pallets per ton',                        'Pallets/Ton',     1.2::NUMERIC,  'Packing',    'PACKLOG', 603),
    ('PACK_LBR_HR_TON', 'Packing labor hours per ton',            'Pack Lbr Hr',     6::NUMERIC,    'Packing',    'PACKLOG', 604),
    ('WAREHOUSE_DAYS',  'Warehouse holding days',                 'WH Days',        14::NUMERIC,    'Logistics',  'PACKLOG', 605),
    ('INT_TRANSPORT',   'Internal transport (IDR / kg)',          'Int Transport',  150::NUMERIC,   'Logistics',  'PACKLOG', 606),
    ('EXP_PACK_PREMIUM','Export packing premium (IDR / kg)',      'Exp Premium',    250::NUMERIC,   'Logistics',  'PACKLOG', 607)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- RATE params -- Extra utilities (6)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    default_value, is_active, created_by, owner_department, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'RATE',
       v.default_value, TRUE, 'seed_000240', v.owner_department, v.display_group, v.display_order
  FROM (VALUES
    ('GAS_RATE_M3',     'Natural gas rate (IDR / m3)',            'Gas Rate',      6500::NUMERIC,   'Finance',    'RATES', 700),
    ('GAS_NM3_PER_KG',  'Natural gas consumption (Nm3/kg)',       'Gas Nm3/kg',      0.4::NUMERIC,  'Engineering','RATES', 701),
    ('CHILL_RATE',      'Chilled water rate (IDR / RTH)',         'Chill Rate',    2500::NUMERIC,   'Finance',    'RATES', 702),
    ('CHILL_PER_KG',    'Chilled water usage (RTH / kg)',         'Chill /kg',       0.05::NUMERIC, 'Engineering','RATES', 703),
    ('CONDENSATE_PCT',  'Condensate recovery percent',            'Condensate %',   70::NUMERIC,    'Engineering','RATES', 704),
    ('LBR_RATE_TECH',   'Technical labor rate (IDR / hr)',        'Tech Lbr Rate', 35000::NUMERIC,  'Finance',    'RATES', 705)
  ) AS v(param_code, param_name, param_short_name, default_value, owner_department, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

-- ============================================================================
-- CALCULATED params -- New sinks (9)
-- ============================================================================
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    is_active, created_by, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'CALCULATED',
       TRUE, 'seed_000240', v.display_group, v.display_order
  FROM (VALUES
    ('COST_DYE_CHEM',   'Cost of dyestuff chemicals (IDR / kg)',  'Cost Dye Chem',   'COST_OUTPUT', 800),
    ('COST_AUX_CHEM',   'Cost of auxiliary chemicals (IDR / kg)', 'Cost Aux Chem',   'COST_OUTPUT', 801),
    ('COST_WATER_DYE',  'Cost of water for dyeing (IDR / kg)',    'Cost Water Dye',  'COST_OUTPUT', 802),
    ('COST_STEAM_DYE',  'Cost of steam for dyeing (IDR / kg)',    'Cost Steam Dye',  'COST_OUTPUT', 803),
    ('COST_GAS',        'Cost of natural gas (IDR / kg)',         'Cost Gas',        'COST_OUTPUT', 804),
    ('COST_CHILLER',    'Cost of chilled water (IDR / kg)',       'Cost Chiller',    'COST_OUTPUT', 805),
    ('COST_INSPECT_QC', 'Cost of inspection / QC (IDR / kg)',     'Cost Insp QC',    'COST_OUTPUT', 806),
    ('COST_PACKING_TOT','Total packing cost (IDR / kg)',          'Cost Packing',    'COST_OUTPUT', 807),
    ('COST_TRANSPORT',  'Cost of transport (IDR / kg)',           'Cost Transport',  'COST_OUTPUT', 808)
  ) AS v(param_code, param_name, param_short_name, display_group, display_order)
 WHERE NOT EXISTS (
       SELECT 1 FROM mst_parameter p WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
 );

COMMIT;
