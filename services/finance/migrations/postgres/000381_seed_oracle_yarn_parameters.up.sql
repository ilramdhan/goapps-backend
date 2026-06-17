-- 000381: Seed Oracle yarn costing parameters.
-- Idempotency: WHERE NOT EXISTS guard (partial unique index prevents ON CONFLICT).
-- UOM categories WEIGHT/ENERGY/TIME/DIMENSIONLESS already exist from 000007 + 000374.

BEGIN;

-- Ensure required UOM codes exist (KG, PCT, DEN, KWH, HR may not exist yet).
INSERT INTO mst_uom (uom_code, uom_name, uom_category_id, created_by)
SELECT 'KG', 'Kilogram',
       (SELECT uom_category_id FROM mst_uom_category WHERE category_code = 'WEIGHT' LIMIT 1),
       'seed_000381'
WHERE NOT EXISTS (SELECT 1 FROM mst_uom WHERE uom_code = 'KG');

INSERT INTO mst_uom (uom_code, uom_name, uom_category_id, created_by)
SELECT 'PCT', 'Percent',
       (SELECT uom_category_id FROM mst_uom_category WHERE category_code = 'DIMENSIONLESS' LIMIT 1),
       'seed_000381'
WHERE NOT EXISTS (SELECT 1 FROM mst_uom WHERE uom_code = 'PCT');

INSERT INTO mst_uom (uom_code, uom_name, uom_category_id, created_by)
SELECT 'DEN', 'Denier',
       (SELECT uom_category_id FROM mst_uom_category WHERE category_code = 'DIMENSIONLESS' LIMIT 1),
       'seed_000381'
WHERE NOT EXISTS (SELECT 1 FROM mst_uom WHERE uom_code = 'DEN');

INSERT INTO mst_uom (uom_code, uom_name, uom_category_id, created_by)
SELECT 'KWH', 'Kilowatt-hour',
       (SELECT uom_category_id FROM mst_uom_category WHERE category_code = 'ENERGY' LIMIT 1),
       'seed_000381'
WHERE NOT EXISTS (SELECT 1 FROM mst_uom WHERE uom_code = 'KWH');

INSERT INTO mst_uom (uom_code, uom_name, uom_category_id, created_by)
SELECT 'HR', 'Hour',
       (SELECT uom_category_id FROM mst_uom_category WHERE category_code = 'TIME' LIMIT 1),
       'seed_000381'
WHERE NOT EXISTS (SELECT 1 FROM mst_uom WHERE uom_code = 'HR');

-- Seed Oracle yarn INPUT/RATE params.
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    default_value, min_value, max_value,
    display_group, display_order, created_by, is_active
)
SELECT v.param_code, v.param_name, v.param_short_name, v.data_type, v.param_category,
       (SELECT uom_id FROM mst_uom WHERE uom_code = v.uom_code LIMIT 1),
       v.owner_department, v.is_required, v.is_period_dep,
       v.default_val, v.min_val, v.max_val,
       v.display_group, v.display_order, 'seed_000381', TRUE
FROM (VALUES
  -- Spec
  ('MC_NAME',         'Machine Code/Name',              'Machine',     'TEXT',   'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   NULL,  NULL,   'Spec',        10),
  ('YARN_DENIER',     'Yarn Nominal Denier',             'Denier',      'NUMBER', 'INPUT',    'DEN', 'Engineering', TRUE,  FALSE, NULL,   0,     9999,   'Spec',        20),
  ('ACT_DENIER',      'Yarn Actual Denier',              'Act Den',     'NUMBER', 'INPUT',    'DEN', 'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Spec',        21),
  ('FILAMENT_COUNT',  'Filament Count',                  'Filaments',   'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   1,     999,    'Spec',        30),
  ('NO_OF_PLY',       'Number of Ply',                   'Ply',         'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, 1,      1,     12,     'Spec',        40),
  ('CROSS_SECTION',   'Cross Section Shape',             'X-Section',   'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'Spec',        50),
  ('LUSTRE_TYPE',     'Lustre Type',                     'Lustre',      'TEXT',   'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   NULL,  NULL,   'Spec',        60),
  ('INTERMINGLE',     'Intermingling Type',              'Intermingle', 'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'Spec',        70),
  ('RM_TYPE',         'Raw Material Type',               'RM Type',     'TEXT',   'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   NULL,  NULL,   'Spec',        80),
  ('Y_TYPE',          'Yarn Type Code',                  'Y Type',      'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'Spec',        90),
  ('POLYMER_IV',      'Polymer IV (dl/g)',               'IV',          'NUMBER', 'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   0,     5,      'Spec',       100),
  ('TPM',             'Twist Per Meter',                 'TPM',         'NUMBER', 'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Spec',       120),
  -- Machine
  ('MC_SPEED',        'Machine Speed (m/min)',           'Speed',       'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   0,     9999,   'Machine',     10),
  ('MC_EFFICIENCY',   'Machine Efficiency (%)',          'Efficiency',  'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, 95,     50,    100,    'Machine',     20),
  ('NO_OF_POSITION',  'Number of Positions',             'Positions',   'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   1,     9999,   'Machine',     30),
  ('NO_OF_END',       'Number of Ends',                  'Ends',        'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, 1,      1,     100,    'Machine',     40),
  ('MACHINE_RPM',     'Machine RPM',                     'RPM',         'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   100,   50000,  'Machine',     50),
  ('CHANGE_OVER_KG',  'Change-Over Loss (kg)',           'CO Loss',     'NUMBER', 'INPUT',    'KG',  'Engineering', FALSE, FALSE, 100,    0,     9999,   'Machine',     60),
  ('VOL_BUCKET_1_QTY','Volume Bucket 1 Qty threshold',  'VB1 Qty',     'NUMBER', 'INPUT',    'KG',  'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Machine',     70),
  ('VOL_BUCKET_2_QTY','Volume Bucket 2 Qty threshold',  'VB2 Qty',     'NUMBER', 'INPUT',    'KG',  'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Machine',     71),
  ('VOL_BUCKET_3_QTY','Volume Bucket 3 Qty threshold',  'VB3 Qty',     'NUMBER', 'INPUT',    'KG',  'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Machine',     72),
  ('VOL_BUCKET_4_QTY','Volume Bucket 4 Qty threshold',  'VB4 Qty',     'NUMBER', 'INPUT',    'KG',  'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Machine',     73),
  ('VOL_BUCKET_5_QTY','Volume Bucket 5 Qty threshold',  'VB5 Qty',     'NUMBER', 'INPUT',    'KG',  'Engineering', FALSE, FALSE, NULL,   0,     9999,   'Machine',     74),
  -- Raw Material
  ('RAW_MATERIAL',    'Raw Material Reference',          'RM Ref',      'TEXT',   'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   NULL,  NULL,   'RawMaterial', 10),
  ('WASTE_PCT',       'Waste Percentage (%)',            'Waste %',     'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, 0.7,    0,     20,     'RawMaterial', 20),
  ('OPU',             'Oil Pick-Up (g/kg)',              'OPU',         'NUMBER', 'INPUT',    NULL,  'Engineering', FALSE, FALSE, 2.2,    0,     50,     'RawMaterial', 30),
  ('OIL_NAME',        'Coning Oil Item Name',            'Oil Name',    'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'RawMaterial', 40),
  -- Grade Distribution
  ('AX_PERC',         'AX Grade Percentage (%)',         'AX %',        'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeDist',   10),
  ('AE_PERC',         'AE Grade Percentage (%)',         'AE %',        'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeDist',   20),
  ('A9_PERC',         'A9 Grade Percentage (%)',         'A9 %',        'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeDist',   30),
  ('A_PERC',          'A Grade Percentage (%)',          'A %',         'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeDist',   40),
  ('B_PERC',          'B Grade Percentage (%)',          'B %',         'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeDist',   50),
  ('C_PERC',          'C Grade Percentage (%)',          'C %',         'NUMBER', 'INPUT',    'PCT', 'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeDist',   60),
  -- Grade Weight INPUT
  ('AX_WT',           'AX Bobbin Weight (kg)',           'AX Wt',       'NUMBER', 'INPUT',    'KG',  'Engineering', TRUE,  FALSE, NULL,   0,     100,    'GradeWeight', 10),
  -- Captive Packing
  ('CAP_PACK_CODE',   'Captive Packing Type Code',      'Cap Pack',    'TEXT',   'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   NULL,  NULL,   'CapPacking',  10),
  ('CAP_NO_OF_BOB',   'Captive No. of Bobbins/Box',     'Cap # Bob',   'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, 6,      1,     100,    'CapPacking',  20),
  ('CAP_BOB_RATE',    'Captive Bobbin Rate (USD/bob)',   'Cap Bob Rate','NUMBER', 'RATE',     NULL,  'Finance',     TRUE,  TRUE,  NULL,   0,     NULL,   'CapPacking',  40),
  ('CAP_BOX_RATE',    'Captive Box Rate (USD/box)',      'Cap Box Rate','NUMBER', 'RATE',     NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'CapPacking',  50),
  -- Delivery Packing
  ('DEL_PACK_CODE',   'Delivery Packing Type Code',     'Del Pack',    'TEXT',   'INPUT',    NULL,  'Engineering', TRUE,  FALSE, NULL,   NULL,  NULL,   'DelPacking',  10),
  ('DEL_NO_OF_BOB',   'Delivery No. of Bobbins/Box',    'Del # Bob',   'NUMBER', 'INPUT',    NULL,  'Engineering', TRUE,  FALSE, 6,      1,     100,    'DelPacking',  20),
  ('DEL_BOB_RATE',    'Delivery Bobbin Rate (USD/bob)',  'Del Bob Rate','NUMBER', 'RATE',     NULL,  'Finance',     TRUE,  TRUE,  NULL,   0,     NULL,   'DelPacking',  40),
  ('DEL_BOX_RATE',    'Delivery Box Rate (USD/box)',     'Del Box Rate','NUMBER', 'RATE',     NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'DelPacking',  50),
  -- Heatset
  ('HEATSET_CODE',    'Heatset Type Code',               'Heatset',     'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'Heatset',     10),
  ('NO_OF_TROLLIES',  'Number of Trolleys',              'Trolleys',    'NUMBER', 'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   0,     999,    'Heatset',     20),
  ('NO_BOB_PER_TROL', 'Bobbins per Trolley',             'Bob/Trol',    'NUMBER', 'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   0,     999,    'Heatset',     30),
  -- Rates
  ('RM_RATE',         'Raw Material Rate/kg (USD)',      'RM Rate',     'NUMBER', 'RATE',     NULL,  'Finance',     TRUE,  TRUE,  NULL,   0,     NULL,   'Rates',       10),
  ('OIL_RATE',        'Coning Oil Rate/kg (USD)',        'Oil Rate',    'NUMBER', 'RATE',     NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Rates',       30),
  ('ELEC_KWH',        'Electricity Consumption kWh/kg', 'Elec kWh',    'NUMBER', 'INPUT',    'KWH', 'Engineering', TRUE,  FALSE, NULL,   0,     50,     'Rates',       50),
  ('ELEC_RATE',       'Electricity Rate (USD/kWh)',      'Elec Rate',   'NUMBER', 'RATE',     NULL,  'Finance',     TRUE,  TRUE,  NULL,   0,     NULL,   'Rates',       51),
  ('LABOR_HRS',       'Labor Hours per kg',              'Labor Hrs',   'NUMBER', 'INPUT',    'HR',  'Engineering', TRUE,  FALSE, NULL,   0,     10,     'Rates',       60),
  ('LABOR_RATE',      'Labor Rate (USD/hour)',            'Labor Rate',  'NUMBER', 'RATE',     NULL,  'Finance',     TRUE,  TRUE,  NULL,   0,     NULL,   'Rates',       61),
  ('DEPREC_PER_KG',   'Depreciation per kg (USD)',       'Depr/kg',     'NUMBER', 'INPUT',    NULL,  'Finance',     TRUE,  FALSE, NULL,   0,     NULL,   'Rates',       70),
  -- Master Batch
  ('MB_CODE',         'Master Batch Code',               'MB Code',     'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'MasterBatch', 10),
  ('MB_DYE_NAME',     'MB Dye Name',                     'MB Dye',      'TEXT',   'INPUT',    NULL,  'Engineering', FALSE, FALSE, NULL,   NULL,  NULL,   'MasterBatch', 20),
  ('MB_DOZING_PCT',   'MB Dozing Rate (%)',              'MB Doz %',    'NUMBER', 'INPUT',    'PCT', 'Engineering', FALSE, FALSE, NULL,   0,     100,    'MasterBatch', 30),
  ('MB_RATE',         'MB Rate (USD/kg)',                 'MB Rate',     'NUMBER', 'RATE',     NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'MasterBatch', 40),
  -- Intermingling
  ('SPECIAL_COST_1',  'Special Cost 1/kg (USD)',         'Spec Cost 1', 'NUMBER', 'INPUT',    NULL,  'Finance',     FALSE, FALSE, NULL,   0,     NULL,   'Intermingling',20),
  ('SPECIAL_COST_2',  'Special Cost 2/kg (USD)',         'Spec Cost 2', 'NUMBER', 'INPUT',    NULL,  'Finance',     FALSE, FALSE, NULL,   0,     NULL,   'Intermingling',30),
  -- Conversion
  ('POWER_PER_DAY',   'Power Cost per Day (USD)',        'Power/day',   'NUMBER', 'INPUT',    NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Conversion',  20),
  ('MANPOWER_PER_DAY','Manpower Cost per Day (USD)',     'Labor/day',   'NUMBER', 'INPUT',    NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Conversion',  30),
  ('OVERHEAD_PER_DAY','Overhead Cost per Day (USD)',     'Overhead/day','NUMBER', 'INPUT',    NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Conversion',  40),
  ('SPARES_PER_DAY',  'Spares & Consumables per Day',   'Spares/day',  'NUMBER', 'INPUT',    NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Conversion',  50),
  ('LABOR_OVERHEAD_PCT','Labor Overhead % (benefits)',  'Labor OH%',   'NUMBER', 'INPUT',    'PCT', 'Finance',     FALSE, FALSE, NULL,   0,     100,    'Conversion', 110),
  -- Quality Loss
  ('STD_LOSS_GRADE',  'Standard Value Loss Grade',       'Std Loss Gr', 'TEXT',   'INPUT',    NULL,  'Finance',     FALSE, FALSE, NULL,   NULL,  NULL,   'QualityLoss', 10),
  ('BC_LOSS_GRADE',   'BC Value Loss Grade Label',       'BC Loss Gr',  'TEXT',   'INPUT',    NULL,  'Finance',     FALSE, FALSE, NULL,   NULL,  NULL,   'QualityLoss', 20),
  ('NON_STD_PERC',    'Non-Standard Production %',       'Non-Std %',   'NUMBER', 'INPUT',    'PCT', 'Finance',     FALSE, FALSE, NULL,   0,     100,    'QualityLoss', 30),
  ('BC_PERC',         'BC Grade Production %',           'BC %',        'NUMBER', 'INPUT',    'PCT', 'Finance',     FALSE, FALSE, NULL,   0,     100,    'QualityLoss', 40),
  ('BC_RECOVERY_RATE','BC Recovery Rate (%)',            'BC Rec %',    'NUMBER', 'INPUT',    'PCT', 'Finance',     FALSE, FALSE, NULL,   0,     100,    'QualityLoss', 50),
  -- Utilities
  ('STEAM_RATE',      'Steam Cost Rate (USD/kg steam)',  'Steam Rate',  'NUMBER', 'RATE',     NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Utilities',   10),
  ('WATER_RATE',      'Water Cost Rate (USD/m3)',        'Water Rate',  'NUMBER', 'RATE',     NULL,  'Finance',     FALSE, TRUE,  NULL,   0,     NULL,   'Utilities',   20),
  -- Material overhead
  ('MAT_OVERHEAD_PCT','Material Overhead Percentage (%)', 'Mat OH%',   'NUMBER', 'INPUT',    'PCT', 'Finance',     FALSE, FALSE, NULL,   0,     100,    'Conversion',  120)
) AS v(param_code, param_name, param_short_name, data_type, param_category,
       uom_code, owner_department, is_required, is_period_dep,
       default_val, min_val, max_val, display_group, display_order)
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter p
    WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
);

-- CALCULATED params
INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    is_active, created_by, display_group, display_order
)
SELECT v.param_code, v.param_name, v.param_short_name, 'NUMBER', 'CALCULATED',
       TRUE, 'seed_000381', v.display_group, v.display_order
FROM (VALUES
  ('DRAW_RATIO',      'Draw Ratio',                      'DR',           'Spec',        110),
  ('RM_NORMS',        'RM Consumption Norms',            'RM Norms',     'RawMaterial',  50),
  ('AE_WT',           'AE Bobbin Weight (kg)',           'AE Wt',        'GradeWeight',  20),
  ('A9_WT',           'A9 Bobbin Weight (kg)',           'A9 Wt',        'GradeWeight',  30),
  ('A_WT',            'A Bobbin Weight (kg)',            'A Wt',         'GradeWeight',  40),
  ('B_WT',            'B Bobbin Weight (kg)',            'B Wt',         'GradeWeight',  50),
  ('C_WT',            'C Bobbin Weight (kg)',            'C Wt',         'GradeWeight',  60),
  ('NET_BOB_WT',      'Net Bobbin Weight (kg)',          'Net Bob Wt',   'GradeWeight',  70),
  ('CAP_BOX_WT',      'Captive Box Weight (kg)',         'Cap Box Wt',   'CapPacking',   30),
  ('CAP_PACK_COST',   'Captive Packing Cost/kg (USD)',   'Cap Pack Cost','CapPacking',   60),
  ('DEL_BOX_WT',      'Delivery Box Weight (kg)',        'Del Box Wt',   'DelPacking',   30),
  ('DEL_PACK_COST',   'Delivery Packing Cost/kg (USD)',  'Del Pack Cost','DelPacking',   60),
  ('BATCH_WEIGHT',    'Batch Weight (kg)',               'Batch Wt',     'Heatset',      40),
  ('RM_LANDED_COST',  'RM Landed Cost/kg (USD)',         'RM LC',        'Rates',        20),
  ('OIL_COST',        'Coning Oil Cost/kg yarn (USD)',   'Oil Cost',     'Rates',        40),
  ('MB_COST',         'MB Cost/kg yarn (USD)',           'MB Cost',      'MasterBatch',  50),
  ('RP_DOZING',       'Recipe Dozing Rate',              'RP Doz',       'MasterBatch',  60),
  ('INTERMINGLE_COST','Intermingling Cost/kg (USD)',     'Intermingle$', 'Intermingling',10),
  ('NET_PRODUCTION',  'Net Production (kg/day)',         'Net Prod',     'Conversion',   10),
  ('POWER_PER_KG',    'Power Cost per kg (USD)',         'Power/kg',     'Conversion',   60),
  ('MANPOWER_PER_KG', 'Manpower Cost per kg (USD)',      'Labor/kg',     'Conversion',   70),
  ('OVERHEAD_PER_KG', 'Overhead per kg (USD)',           'Overhead/kg',  'Conversion',   80),
  ('SPARES_PER_KG',   'Spares & Consumables per kg',    'Spares/kg',    'Conversion',   90),
  ('TOTAL_FIXED_COST','Total Fixed Cost per kg (USD)',   'Fixed Cost/kg','Conversion',  100),
  ('CONV_CAP_EX_MB',  'Conv + Captive Pack excl MB',    'Conv Cap',     'Subtotals',    10),
  ('CONV_DEL_EX_MB',  'Conv + Delivery Pack excl MB',   'Conv Del',     'Subtotals',    20),
  ('CAP_COST_PRE_QL', 'Captive Cost Before Qloss',      'Cap Pre-QL',   'Subtotals',    30),
  ('DEL_COST_PRE_QL', 'Delivery Cost Before Qloss',     'Del Pre-QL',   'Subtotals',    40),
  ('NON_STD_LOSS',    'Non-Standard Value Loss/kg',      'Non-Std Loss', 'QualityLoss',  60),
  ('BC_LOSS_CAP',     'BC Value Loss Captive/kg',        'BC Loss Cap',  'QualityLoss',  70),
  ('BC_LOSS_DEL',     'BC Value Loss Delivery/kg',       'BC Loss Del',  'QualityLoss',  80),
  ('QLOSS_CAP',       'Quality Loss Captive/kg',         'QLoss Cap',    'QualityLoss',  90),
  ('QLOSS_DEL',       'Quality Loss Delivery/kg',        'QLoss Del',    'QualityLoss', 100),
  ('COST_CAP_FINAL',  'Final Captive Cost/kg (USD)',     'Cap Final',    'CostOutput',   10),
  ('COST_DEL_FINAL',  'Final Delivery Cost/kg (USD)',    'Del Final',    'CostOutput',   20),
  ('VB1_LOSS',        'VB1 Change-Over Loss/kg',         'VB1 Loss',     'VolumeBucket', 10),
  ('VB2_LOSS',        'VB2 Change-Over Loss/kg',         'VB2 Loss',     'VolumeBucket', 20),
  ('VB3_LOSS',        'VB3 Change-Over Loss/kg',         'VB3 Loss',     'VolumeBucket', 30),
  ('VB4_LOSS',        'VB4 Change-Over Loss/kg',         'VB4 Loss',     'VolumeBucket', 40),
  ('VB5_LOSS',        'VB5 Change-Over Loss/kg',         'VB5 Loss',     'VolumeBucket', 50),
  ('VB1_DEL_COST',    'VB1 Delivery Cost/kg',            'VB1 Cost',     'VolumeBucket', 60),
  ('VB2_DEL_COST',    'VB2 Delivery Cost/kg',            'VB2 Cost',     'VolumeBucket', 70),
  ('VB3_DEL_COST',    'VB3 Delivery Cost/kg',            'VB3 Cost',     'VolumeBucket', 80),
  ('VB4_DEL_COST',    'VB4 Delivery Cost/kg',            'VB4 Cost',     'VolumeBucket', 90),
  ('VB5_DEL_COST',    'VB5 Delivery Cost/kg',            'VB5 Cost',     'VolumeBucket',100),
  ('HEATSET_COST_KG', 'Heatset Cost per kg (USD)',       'Heatset/kg',   'Heatset',      50),
  ('COST_ELEC',       'Electricity Cost per kg (USD)',   'Cost Elec',    'Conversion',   62),
  ('COST_LABOR',      'Labor Cost per kg (USD)',         'Cost Labor',   'Conversion',   72),
  ('COST_CONVERSION', 'Total Conversion Cost per kg',   'Conv Total',   'Conversion',  105),
  ('COST_STAGE_OUT',  'Final Stage Cost (engine sink)',  'Stage Out',    'CostOutput',   99)
) AS v(param_code, param_name, param_short_name, display_group, display_order)
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter p
    WHERE p.param_code = v.param_code AND p.deleted_at IS NULL
);

COMMIT;
