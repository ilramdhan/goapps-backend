-- 000236: Realistic textile products + DAG routes + CAPP (S8e-fix 3/3).
--
-- Seeds ~30 cost_product_master rows (TXFX_ prefix) modelling an Indonesian
-- polyester / textile mill across 6 levels:
--     Raw materials (PTA, MEG, dyestuffs - via cst_rm_cost as ITEM RMs)
--   Level 5: Polyester Chip (3 lustres) - 1 stage from RM
--   Level 4: POY (5 deniers/lustres)     - from Chip
--   Level 3: DTY (6 variants, demonstrates SPLIT from shared POY)
--   Level 2: Greige fabric (4 SKUs, demonstrates MERGE of 2 DTY yarns)
--   Level 1: Dyed fabric (5 SKUs)
--   Level 0: FG fabric + FG cone yarn (6 SKUs)
--
-- Each non-RM product has:
--   * one cost_route_head (status COMPLETE, version 1)
--   * one cost_route_seq (level 1, seq 1)
--   * 1..4 cost_route_rm rows (PRODUCT RMs for upstream textile products,
--     ITEM RMs for purchased chemicals)
--   * full CAPP rows for the engaged INPUT + RATE params
--   * cost_product_parameter values for those params with realistic IDR rates
--
-- Idempotency: every INSERT either uses ON CONFLICT (on real unique
-- constraints) or NOT EXISTS guards. Replayable.
--
-- DEVIATIONS from the S8e-fix brief:
--   * Param codes abbreviated to fit VARCHAR(20). The engine reads
--     COST_RM_TOTAL (auto-populated upstream RM cost) and COST_STAGE_OUT
--     (the existing chain's terminal sink). Both unchanged.
--   * cpm_product_code limited to VARCHAR(20), names shortened accordingly.
--   * Reused existing canonical params/formulas wherever they already
--     covered the same semantics (WASTE_PCT, LABOR_*, ELEC_*, etc.).

BEGIN;

-- =============================================================================
-- 0. Helper temp table mapping logical product codes -> resolved sys_ids
--    + level for downstream lookups. Built per migration run.
-- =============================================================================
CREATE TEMP TABLE _txfx_products (
    code              VARCHAR(20) PRIMARY KEY,
    name              TEXT NOT NULL,
    product_type_code VARCHAR(5) NOT NULL,
    level             INT NOT NULL,
    sys_id            BIGINT
);

INSERT INTO _txfx_products (code, name, product_type_code, level) VALUES
    -- Level 5: Polymer chip (RM only)
    ('TXFX_CHIP_SD',       'Polyester Chip Semi-Dull',           'INTER', 5),
    ('TXFX_CHIP_BR',       'Polyester Chip Bright',              'INTER', 5),
    ('TXFX_CHIP_FD',       'Polyester Chip Full-Dull',           'INTER', 5),
    -- Level 4: POY (from Chip)
    ('TXFX_POY_075SD',     'POY 75D/36F Semi-Dull',              'POY',   4),
    ('TXFX_POY_150SD',     'POY 150D/48F Semi-Dull',             'POY',   4),
    ('TXFX_POY_300SD',     'POY 300D/96F Semi-Dull',             'POY',   4),
    ('TXFX_POY_075BR',     'POY 75D/36F Bright',                 'POY',   4),
    ('TXFX_POY_150BR',     'POY 150D/48F Bright',                'POY',   4),
    -- Level 3: DTY (SPLIT: shared POY -> multiple DTY variants)
    ('TXFX_DTY_075SD_NIM', 'DTY 75D/36F SD Non-Intermingled',    'DTY',   3),
    ('TXFX_DTY_075SD_IM',  'DTY 75D/36F SD Intermingled',        'DTY',   3),
    ('TXFX_DTY_150SD_NIM', 'DTY 150D/48F SD Non-Intermingled',   'DTY',   3),
    ('TXFX_DTY_150SD_IM',  'DTY 150D/48F SD Intermingled',       'DTY',   3),
    ('TXFX_DTY_300SD',     'DTY 300D/96F SD',                    'DTY',   3),
    ('TXFX_DTY_075BR',     'DTY 75D/36F Bright',                 'DTY',   3),
    -- Level 2: Greige fabric (MERGE: multiple DTY yarns)
    ('TXFX_GRG_JRS_180',   'Greige Jersey 180gsm',               'INTER', 2),
    ('TXFX_GRG_TRC_120',   'Greige Tricot 120gsm',               'INTER', 2),
    ('TXFX_GRG_RIB_220',   'Greige Rib 220gsm',                  'INTER', 2),
    ('TXFX_GRG_SPC_280',   'Greige Spacer 280gsm',               'INTER', 2),
    -- Level 1: Dyed fabric
    ('TXFX_DYE_JRS_BLK',   'Dyed Jersey 180gsm Black',           'INTER', 1),
    ('TXFX_DYE_JRS_NVY',   'Dyed Jersey 180gsm Navy',            'INTER', 1),
    ('TXFX_DYE_TRC_RED',   'Dyed Tricot 120gsm Red',             'INTER', 1),
    ('TXFX_DYE_RIB_WHT',   'Dyed Rib 220gsm Optical White',      'INTER', 1),
    ('TXFX_DYE_SPC_GRY',   'Dyed Spacer 280gsm Grey',            'INTER', 1),
    -- Level 0: FG (sellable)
    ('TXFX_FG_JRS_BLK',    'FG Jersey Black 180gsm',             'FG',    0),
    ('TXFX_FG_JRS_NVY',    'FG Jersey Navy 180gsm',              'FG',    0),
    ('TXFX_FG_TRC_RED',    'FG Tricot Red 120gsm',               'FG',    0),
    ('TXFX_FG_RIB_WHT',    'FG Rib Optical White 220gsm',        'FG',    0),
    ('TXFX_FG_SPC_GRY',    'FG Spacer Grey 280gsm',              'FG',    0),
    ('TXFX_FG_DTY_75_IM',  'FG DTY 75D SD IM (cone)',            'FG',    0);

-- =============================================================================
-- 1. Insert cost_product_master rows (idempotent via NOT EXISTS).
-- =============================================================================
INSERT INTO cost_product_master (
    cpm_product_code, cpm_product_name, cpm_product_type_id, cpm_grade_code,
    cpm_description, cpm_is_active, cpm_created_by, cpm_updated_by
)
SELECT v.code, v.name, ct.cpt_type_id, 'AX',
       'S8e-fix textile fixture seed', TRUE, 'seed_000236', 'seed_000236'
  FROM _txfx_products v
  JOIN cost_product_type ct ON ct.cpt_type_code = v.product_type_code
 WHERE NOT EXISTS (
       SELECT 1 FROM cost_product_master cpm WHERE cpm.cpm_product_code = v.code
 );

-- Resolve sys_id back into the temp table for downstream use.
UPDATE _txfx_products t
   SET sys_id = cpm.cpm_product_sys_id
  FROM cost_product_master cpm
 WHERE cpm.cpm_product_code = t.code;

-- =============================================================================
-- 2. Seed cst_rm_cost rows for the ITEM RMs referenced by routes.
--    Period 202604. ON CONFLICT (period, rm_code) DO NOTHING preserves any
--    existing pre-seeded data. Flags + flag_*_used set to CONS (the most
--    common "real" flag).
-- =============================================================================
INSERT INTO cst_rm_cost (
    period, rm_code, rm_type, item_code, rm_name, uom_code,
    cons_rate, cost_val, cost_mark, cost_sim,
    flag_valuation, flag_marketing, flag_simulation,
    flag_valuation_used, flag_marketing_used, flag_simulation_used,
    calculated_at, calculated_by, created_by
)
SELECT '202604', v.rm_code, 'ITEM', v.rm_code, v.rm_name, 'KG',
       v.rate, v.rate, v.rate, v.rate,
       'CONS','CONS','CONS','CONS','CONS','CONS',
       NOW(), 'seed_000236', 'seed_000236'
  FROM (VALUES
    ('TXFX_PTA',          'PTA Purified Terephthalic Acid',  12000),
    ('TXFX_MEG',          'MEG Mono Ethylene Glycol',        11000),
    ('TXFX_SPIN_OIL',     'Spin Finish Oil',                 25000),
    ('TXFX_TIO2',         'Titanium Dioxide (delustrant)',   42000),
    ('TXFX_DYE_BLK',      'Disperse Dyestuff Black',        180000),
    ('TXFX_DYE_NVY',      'Disperse Dyestuff Navy',         165000),
    ('TXFX_DYE_RED',      'Disperse Dyestuff Red',          195000),
    ('TXFX_DYE_WHT',      'Optical Brightener White',       240000),
    ('TXFX_DYE_GRY',      'Disperse Dyestuff Grey',         150000),
    ('TXFX_NAOH',         'Caustic Soda 50%',                 4500),
    ('TXFX_AC_ACID',      'Acetic Acid',                     12000),
    ('TXFX_SODA_ASH',     'Soda Ash',                         5000),
    ('TXFX_CONE',         'Paper Cone Tube',                  1500),
    ('TXFX_LEVELING',     'Leveling Agent',                  38000)
  ) AS v(rm_code, rm_name, rate)
ON CONFLICT (period, rm_code) DO NOTHING;

-- =============================================================================
-- 3. Build routes (head + seq + rm) for every non-RM-only product.
--    Strategy: PL/pgSQL loop over a (product_code, rm_definitions) list.
-- =============================================================================
DO $$
DECLARE
    r              RECORD;
    v_head_id      BIGINT;
    v_seq_id       BIGINT;
    v_product_id   BIGINT;
    v_rm_product_id BIGINT;
    rm             RECORD;
BEGIN
    FOR r IN
        SELECT * FROM (VALUES
            -- format: (product_code, route_name)
            ('TXFX_CHIP_SD',       'Chip Semi-Dull polymerization'),
            ('TXFX_CHIP_BR',       'Chip Bright polymerization'),
            ('TXFX_CHIP_FD',       'Chip Full-Dull polymerization'),
            ('TXFX_POY_075SD',     'POY 75D Semi-Dull spinning'),
            ('TXFX_POY_150SD',     'POY 150D Semi-Dull spinning'),
            ('TXFX_POY_300SD',     'POY 300D Semi-Dull spinning'),
            ('TXFX_POY_075BR',     'POY 75D Bright spinning'),
            ('TXFX_POY_150BR',     'POY 150D Bright spinning'),
            ('TXFX_DTY_075SD_NIM', 'DTY 75D NIM texturing'),
            ('TXFX_DTY_075SD_IM',  'DTY 75D IM texturing'),
            ('TXFX_DTY_150SD_NIM', 'DTY 150D NIM texturing'),
            ('TXFX_DTY_150SD_IM',  'DTY 150D IM texturing'),
            ('TXFX_DTY_300SD',     'DTY 300D texturing'),
            ('TXFX_DTY_075BR',     'DTY 75D Bright texturing'),
            ('TXFX_GRG_JRS_180',   'Greige Jersey knitting'),
            ('TXFX_GRG_TRC_120',   'Greige Tricot knitting'),
            ('TXFX_GRG_RIB_220',   'Greige Rib knitting'),
            ('TXFX_GRG_SPC_280',   'Greige Spacer knitting'),
            ('TXFX_DYE_JRS_BLK',   'Dyeing Jersey Black'),
            ('TXFX_DYE_JRS_NVY',   'Dyeing Jersey Navy'),
            ('TXFX_DYE_TRC_RED',   'Dyeing Tricot Red'),
            ('TXFX_DYE_RIB_WHT',   'Dyeing Rib White'),
            ('TXFX_DYE_SPC_GRY',   'Dyeing Spacer Grey'),
            ('TXFX_FG_JRS_BLK',    'Finishing+packing Jersey Black'),
            ('TXFX_FG_JRS_NVY',    'Finishing+packing Jersey Navy'),
            ('TXFX_FG_TRC_RED',    'Finishing+packing Tricot Red'),
            ('TXFX_FG_RIB_WHT',    'Finishing+packing Rib White'),
            ('TXFX_FG_SPC_GRY',    'Finishing+packing Spacer Grey'),
            ('TXFX_FG_DTY_75_IM',  'Cone packing + QC for DTY 75D IM')
        ) AS x(code, route_name)
    LOOP
        SELECT sys_id INTO v_product_id FROM _txfx_products WHERE code = r.code;
        IF v_product_id IS NULL THEN
            RAISE NOTICE 'skipping route for %, product not seeded', r.code;
            CONTINUE;
        END IF;

        -- Skip if already has an active route head.
        SELECT crh_head_id INTO v_head_id
          FROM cost_route_head
         WHERE crh_product_sys_id = v_product_id
           AND crh_deleted_at IS NULL
           AND crh_routing_status <> 'LOCKED'
         LIMIT 1;

        IF v_head_id IS NOT NULL THEN
            CONTINUE;
        END IF;

        INSERT INTO cost_route_head (
            crh_product_sys_id, crh_routing_status, crh_version,
            crh_notes, crh_created_by, crh_updated_by
        ) VALUES (
            v_product_id, 'COMPLETE', 1,
            r.route_name, 'seed_000236', 'seed_000236'
        ) RETURNING crh_head_id INTO v_head_id;

        INSERT INTO cost_route_seq (
            crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
            crs_route_name, crs_created_by, crs_updated_by
        ) VALUES (
            v_head_id, v_product_id, 1, 1,
            r.route_name, 'seed_000236', 'seed_000236'
        ) RETURNING crs_seq_id INTO v_seq_id;

        -- Now insert route RM rows per product.
        FOR rm IN
            SELECT * FROM (VALUES
                -- format: (parent_product, rm_type, rm_ref_code, ratio, sub_type)
                -- ----- CHIP from RM -----
                ('TXFX_CHIP_SD',       'ITEM',    'TXFX_PTA',          0.860, NULL),
                ('TXFX_CHIP_SD',       'ITEM',    'TXFX_MEG',          0.350, NULL),
                ('TXFX_CHIP_SD',       'ITEM',    'TXFX_SPIN_OIL',     0.005, NULL),
                ('TXFX_CHIP_SD',       'ITEM',    'TXFX_TIO2',         0.003, NULL),
                ('TXFX_CHIP_BR',       'ITEM',    'TXFX_PTA',          0.860, NULL),
                ('TXFX_CHIP_BR',       'ITEM',    'TXFX_MEG',          0.350, NULL),
                ('TXFX_CHIP_BR',       'ITEM',    'TXFX_SPIN_OIL',     0.005, NULL),
                ('TXFX_CHIP_FD',       'ITEM',    'TXFX_PTA',          0.860, NULL),
                ('TXFX_CHIP_FD',       'ITEM',    'TXFX_MEG',          0.350, NULL),
                ('TXFX_CHIP_FD',       'ITEM',    'TXFX_SPIN_OIL',     0.005, NULL),
                ('TXFX_CHIP_FD',       'ITEM',    'TXFX_TIO2',         0.008, NULL),
                -- ----- POY from Chip -----
                ('TXFX_POY_075SD',     'PRODUCT', 'TXFX_CHIP_SD',      1.020, NULL),
                ('TXFX_POY_150SD',     'PRODUCT', 'TXFX_CHIP_SD',      1.020, NULL),
                ('TXFX_POY_300SD',     'PRODUCT', 'TXFX_CHIP_SD',      1.020, NULL),
                ('TXFX_POY_075BR',     'PRODUCT', 'TXFX_CHIP_BR',      1.020, NULL),
                ('TXFX_POY_150BR',     'PRODUCT', 'TXFX_CHIP_BR',      1.020, NULL),
                -- ----- DTY from POY (SPLIT: one POY -> two DTY variants) -----
                ('TXFX_DTY_075SD_NIM', 'PRODUCT', 'TXFX_POY_075SD',    1.018, NULL),
                ('TXFX_DTY_075SD_IM',  'PRODUCT', 'TXFX_POY_075SD',    1.018, NULL),
                ('TXFX_DTY_150SD_NIM', 'PRODUCT', 'TXFX_POY_150SD',    1.018, NULL),
                ('TXFX_DTY_150SD_IM',  'PRODUCT', 'TXFX_POY_150SD',    1.018, NULL),
                ('TXFX_DTY_300SD',     'PRODUCT', 'TXFX_POY_300SD',    1.018, NULL),
                ('TXFX_DTY_075BR',     'PRODUCT', 'TXFX_POY_075BR',    1.018, NULL),
                -- ----- Greige knitting (MERGE: multi DTY -> one fabric) -----
                ('TXFX_GRG_JRS_180',   'PRODUCT', 'TXFX_DTY_150SD_NIM', 0.650, NULL),
                ('TXFX_GRG_JRS_180',   'PRODUCT', 'TXFX_DTY_075SD_NIM', 0.370, NULL),
                ('TXFX_GRG_TRC_120',   'PRODUCT', 'TXFX_DTY_075SD_IM',  1.030, NULL),
                ('TXFX_GRG_RIB_220',   'PRODUCT', 'TXFX_DTY_150SD_IM',  0.700, NULL),
                ('TXFX_GRG_RIB_220',   'PRODUCT', 'TXFX_DTY_300SD',     0.320, NULL),
                ('TXFX_GRG_SPC_280',   'PRODUCT', 'TXFX_DTY_150SD_NIM', 0.620, NULL),
                ('TXFX_GRG_SPC_280',   'PRODUCT', 'TXFX_DTY_075BR',     0.400, NULL),
                -- ----- Dyed from Greige + dyestuff + auxiliaries -----
                ('TXFX_DYE_JRS_BLK',   'PRODUCT', 'TXFX_GRG_JRS_180',   1.050, NULL),
                ('TXFX_DYE_JRS_BLK',   'ITEM',    'TXFX_DYE_BLK',       0.040, NULL),
                ('TXFX_DYE_JRS_BLK',   'ITEM',    'TXFX_NAOH',          0.060, NULL),
                ('TXFX_DYE_JRS_BLK',   'ITEM',    'TXFX_AC_ACID',       0.005, NULL),
                ('TXFX_DYE_JRS_BLK',   'ITEM',    'TXFX_LEVELING',      0.010, NULL),
                ('TXFX_DYE_JRS_NVY',   'PRODUCT', 'TXFX_GRG_JRS_180',   1.050, NULL),
                ('TXFX_DYE_JRS_NVY',   'ITEM',    'TXFX_DYE_NVY',       0.035, NULL),
                ('TXFX_DYE_JRS_NVY',   'ITEM',    'TXFX_NAOH',          0.060, NULL),
                ('TXFX_DYE_JRS_NVY',   'ITEM',    'TXFX_AC_ACID',       0.005, NULL),
                ('TXFX_DYE_TRC_RED',   'PRODUCT', 'TXFX_GRG_TRC_120',   1.050, NULL),
                ('TXFX_DYE_TRC_RED',   'ITEM',    'TXFX_DYE_RED',       0.030, NULL),
                ('TXFX_DYE_TRC_RED',   'ITEM',    'TXFX_NAOH',          0.050, NULL),
                ('TXFX_DYE_TRC_RED',   'ITEM',    'TXFX_AC_ACID',       0.004, NULL),
                ('TXFX_DYE_RIB_WHT',   'PRODUCT', 'TXFX_GRG_RIB_220',   1.045, NULL),
                ('TXFX_DYE_RIB_WHT',   'ITEM',    'TXFX_DYE_WHT',       0.015, NULL),
                ('TXFX_DYE_RIB_WHT',   'ITEM',    'TXFX_NAOH',          0.040, NULL),
                ('TXFX_DYE_SPC_GRY',   'PRODUCT', 'TXFX_GRG_SPC_280',   1.045, NULL),
                ('TXFX_DYE_SPC_GRY',   'ITEM',    'TXFX_DYE_GRY',       0.025, NULL),
                ('TXFX_DYE_SPC_GRY',   'ITEM',    'TXFX_NAOH',          0.050, NULL),
                ('TXFX_DYE_SPC_GRY',   'ITEM',    'TXFX_AC_ACID',       0.005, NULL),
                -- ----- FG fabric from Dyed (packing + finishing) -----
                ('TXFX_FG_JRS_BLK',    'PRODUCT', 'TXFX_DYE_JRS_BLK',   1.020, NULL),
                ('TXFX_FG_JRS_NVY',    'PRODUCT', 'TXFX_DYE_JRS_NVY',   1.020, NULL),
                ('TXFX_FG_TRC_RED',    'PRODUCT', 'TXFX_DYE_TRC_RED',   1.020, NULL),
                ('TXFX_FG_RIB_WHT',    'PRODUCT', 'TXFX_DYE_RIB_WHT',   1.020, NULL),
                ('TXFX_FG_SPC_GRY',    'PRODUCT', 'TXFX_DYE_SPC_GRY',   1.020, NULL),
                -- ----- FG yarn (cone-packed) from DTY -----
                ('TXFX_FG_DTY_75_IM',  'PRODUCT', 'TXFX_DTY_075SD_IM',  1.005, NULL),
                ('TXFX_FG_DTY_75_IM',  'ITEM',    'TXFX_CONE',          0.020, NULL)
            ) AS x(parent_code, rm_type, rm_ref, ratio, sub_type)
            WHERE x.parent_code = r.code
        LOOP
            v_rm_product_id := NULL;
            IF rm.rm_type = 'PRODUCT' THEN
                SELECT sys_id INTO v_rm_product_id FROM _txfx_products WHERE code = rm.rm_ref;
                IF v_rm_product_id IS NULL THEN
                    RAISE EXCEPTION 'unresolved PRODUCT rm reference: % for parent %', rm.rm_ref, r.code;
                END IF;
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_product_sys_id, crm_rm_type, crm_route_rm_name,
                    crm_route_rm_ratio, crm_sub_type,
                    crm_created_by, crm_updated_by
                ) VALUES (
                    v_seq_id, v_product_id,
                    v_rm_product_id, 'PRODUCT', rm.rm_ref,
                    rm.ratio, rm.sub_type,
                    'seed_000236', 'seed_000236'
                );
            ELSE
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_item_code, crm_rm_type, crm_route_rm_name, crm_route_rm_item_code,
                    crm_route_rm_ratio, crm_sub_type,
                    crm_created_by, crm_updated_by
                ) VALUES (
                    v_seq_id, v_product_id,
                    rm.rm_ref, 'ITEM', rm.rm_ref, rm.rm_ref,
                    rm.ratio, rm.sub_type,
                    'seed_000236', 'seed_000236'
                );
            END IF;
        END LOOP;
    END LOOP;
END $$;

-- =============================================================================
-- 4. CAPP rows + cost_product_parameter values for every TXFX product.
--    Strategy: for each product LEVEL we define an "applicability bundle" of
--    (param_code, value) pairs. The bundle reflects realistic 2026 Indonesian
--    polyester / textile economics in IDR.
--
--    Bundles:
--      L5 Chip      : RM-heavy, energy/steam intensive polymerization.
--      L4 POY       : spinning - high spindle/labor + compressed air.
--      L3 DTY       : texturing - high machine + packing + QC.
--      L2 Greige    : knitting - low energy, moderate labor.
--      L1 Dyed      : dyeing - steam/water intensive, high overhead.
--      L0 FG        : adds packing + QC + margin on top.
--
--    All CALCULATED params engaged by the engine chain are also CAPP'd
--    (no value, just applicability).
-- =============================================================================
CREATE TEMP TABLE _txfx_capp_bundle (
    level       INT NOT NULL,
    param_code  VARCHAR(20) NOT NULL,
    value       NUMERIC(20,6)  -- NULL = CALCULATED, applicability only
);

-- Bundle for level 5 (Chip)
INSERT INTO _txfx_capp_bundle (level, param_code, value) VALUES
    (5, 'WASTE_PCT',          1.5),
    (5, 'YIELD_PCT',          98.5),
    (5, 'LABOR_HRS',          0.0012),  -- hrs per kg
    (5, 'LABOR_RATE',         22000),
    (5, 'LABOR_OVERHEAD_PCT', 25),
    (5, 'IND_LABOR_PCT',      25),
    (5, 'MAT_OVERHEAD_PCT',   3),
    (5, 'MACHINE_PER_KG',     1800),
    (5, 'DEPREC_PER_KG',      450),
    (5, 'ELEC_KWH',           0.8),
    (5, 'ELEC_RATE',          1500),
    (5, 'STEAM_KG',           2.5),
    (5, 'STEAM_RATE',         800),
    (5, 'WATER_M3',           0.003),
    (5, 'WATER_RATE',         12000),
    (5, 'AIR_PER_KG',         80),
    (5, 'MAINT_PER_KG',       250),
    (5, 'FACTORY_OH',         600),
    (5, 'QC_PER_KG',          30),
    (5, 'PACK_PER_KG',        0),
    -- calculated, applicability only
    (5, 'COST_RM_TOTAL',      NULL),
    (5, 'COST_RM_LOADED',     NULL),
    (5, 'COST_ELEC',          NULL),
    (5, 'COST_LABOR',         NULL),
    (5, 'COST_LABOR_FULL',    NULL),
    (5, 'COST_CONVERSION',    NULL),
    (5, 'COST_STAGE_OUT',     NULL),
    (5, 'COST_STEAM',         NULL),
    (5, 'COST_WATER',         NULL),
    (5, 'COST_UTIL',          NULL),
    (5, 'COST_OVERHEAD',      NULL),
    (5, 'COST_AFTER_YLD',     NULL);

-- Bundle for level 4 (POY - spinning)
INSERT INTO _txfx_capp_bundle (level, param_code, value) VALUES
    (4, 'WASTE_PCT',          2.0),
    (4, 'YIELD_PCT',          96.5),
    (4, 'LABOR_HRS',          0.0035),
    (4, 'LABOR_RATE',         22000),
    (4, 'LABOR_OVERHEAD_PCT', 30),
    (4, 'IND_LABOR_PCT',      30),
    (4, 'MAT_OVERHEAD_PCT',   3),
    (4, 'MACHINE_PER_KG',     4200),
    (4, 'DEPREC_PER_KG',      850),
    (4, 'ELEC_KWH',           1.8),
    (4, 'ELEC_RATE',          1500),
    (4, 'STEAM_KG',           0.5),
    (4, 'STEAM_RATE',         800),
    (4, 'WATER_M3',           0.002),
    (4, 'WATER_RATE',         12000),
    (4, 'AIR_PER_KG',         220),
    (4, 'MAINT_PER_KG',       380),
    (4, 'FACTORY_OH',         900),
    (4, 'QC_PER_KG',          80),
    (4, 'PACK_PER_KG',        0),
    (4, 'COST_RM_TOTAL',      NULL),
    (4, 'COST_RM_LOADED',     NULL),
    (4, 'COST_ELEC',          NULL),
    (4, 'COST_LABOR',         NULL),
    (4, 'COST_LABOR_FULL',    NULL),
    (4, 'COST_CONVERSION',    NULL),
    (4, 'COST_STAGE_OUT',     NULL),
    (4, 'COST_STEAM',         NULL),
    (4, 'COST_WATER',         NULL),
    (4, 'COST_UTIL',          NULL),
    (4, 'COST_OVERHEAD',      NULL),
    (4, 'COST_AFTER_YLD',     NULL);

-- Bundle for level 3 (DTY - texturing)
INSERT INTO _txfx_capp_bundle (level, param_code, value) VALUES
    (3, 'WASTE_PCT',          1.8),
    (3, 'YIELD_PCT',          97.5),
    (3, 'LABOR_HRS',          0.0045),
    (3, 'LABOR_RATE',         22000),
    (3, 'LABOR_OVERHEAD_PCT', 30),
    (3, 'IND_LABOR_PCT',      28),
    (3, 'MAT_OVERHEAD_PCT',   3),
    (3, 'MACHINE_PER_KG',     5500),
    (3, 'DEPREC_PER_KG',      1100),
    (3, 'ELEC_KWH',           1.2),
    (3, 'ELEC_RATE',          1500),
    (3, 'STEAM_KG',           0),
    (3, 'STEAM_RATE',         800),
    (3, 'WATER_M3',           0.001),
    (3, 'WATER_RATE',         12000),
    (3, 'AIR_PER_KG',         300),
    (3, 'MAINT_PER_KG',       450),
    (3, 'FACTORY_OH',         1100),
    (3, 'QC_PER_KG',          120),
    (3, 'PACK_PER_KG',        250),
    (3, 'COST_RM_TOTAL',      NULL),
    (3, 'COST_RM_LOADED',     NULL),
    (3, 'COST_ELEC',          NULL),
    (3, 'COST_LABOR',         NULL),
    (3, 'COST_LABOR_FULL',    NULL),
    (3, 'COST_CONVERSION',    NULL),
    (3, 'COST_STAGE_OUT',     NULL),
    (3, 'COST_STEAM',         NULL),
    (3, 'COST_WATER',         NULL),
    (3, 'COST_UTIL',          NULL),
    (3, 'COST_OVERHEAD',      NULL),
    (3, 'COST_AFTER_YLD',     NULL);

-- Bundle for level 2 (Greige - knitting)
INSERT INTO _txfx_capp_bundle (level, param_code, value) VALUES
    (2, 'WASTE_PCT',          3.0),
    (2, 'YIELD_PCT',          96.0),
    (2, 'LABOR_HRS',          0.0075),
    (2, 'LABOR_RATE',         22000),
    (2, 'LABOR_OVERHEAD_PCT', 25),
    (2, 'IND_LABOR_PCT',      25),
    (2, 'MAT_OVERHEAD_PCT',   2),
    (2, 'MACHINE_PER_KG',     3800),
    (2, 'DEPREC_PER_KG',      700),
    (2, 'ELEC_KWH',           0.6),
    (2, 'ELEC_RATE',          1500),
    (2, 'STEAM_KG',           0),
    (2, 'STEAM_RATE',         800),
    (2, 'WATER_M3',           0),
    (2, 'WATER_RATE',         12000),
    (2, 'AIR_PER_KG',         100),
    (2, 'MAINT_PER_KG',       280),
    (2, 'FACTORY_OH',         850),
    (2, 'QC_PER_KG',          90),
    (2, 'PACK_PER_KG',        0),
    (2, 'COST_RM_TOTAL',      NULL),
    (2, 'COST_RM_LOADED',     NULL),
    (2, 'COST_ELEC',          NULL),
    (2, 'COST_LABOR',         NULL),
    (2, 'COST_LABOR_FULL',    NULL),
    (2, 'COST_CONVERSION',    NULL),
    (2, 'COST_STAGE_OUT',     NULL),
    (2, 'COST_STEAM',         NULL),
    (2, 'COST_WATER',         NULL),
    (2, 'COST_UTIL',          NULL),
    (2, 'COST_OVERHEAD',      NULL),
    (2, 'COST_AFTER_YLD',     NULL);

-- Bundle for level 1 (Dyed - dyeing/finishing)
INSERT INTO _txfx_capp_bundle (level, param_code, value) VALUES
    (1, 'WASTE_PCT',          4.0),
    (1, 'YIELD_PCT',          94.5),
    (1, 'LABOR_HRS',          0.009),
    (1, 'LABOR_RATE',         22000),
    (1, 'LABOR_OVERHEAD_PCT', 30),
    (1, 'IND_LABOR_PCT',      30),
    (1, 'MAT_OVERHEAD_PCT',   4),
    (1, 'MACHINE_PER_KG',     4500),
    (1, 'DEPREC_PER_KG',      900),
    (1, 'ELEC_KWH',           0.9),
    (1, 'ELEC_RATE',          1500),
    (1, 'STEAM_KG',           4.2),
    (1, 'STEAM_RATE',         800),
    (1, 'WATER_M3',           0.12),
    (1, 'WATER_RATE',         12000),
    (1, 'AIR_PER_KG',         150),
    (1, 'MAINT_PER_KG',       380),
    (1, 'FACTORY_OH',         1500),
    (1, 'QC_PER_KG',          220),
    (1, 'PACK_PER_KG',        0),
    (1, 'COST_RM_TOTAL',      NULL),
    (1, 'COST_RM_LOADED',     NULL),
    (1, 'COST_ELEC',          NULL),
    (1, 'COST_LABOR',         NULL),
    (1, 'COST_LABOR_FULL',    NULL),
    (1, 'COST_CONVERSION',    NULL),
    (1, 'COST_STAGE_OUT',     NULL),
    (1, 'COST_STEAM',         NULL),
    (1, 'COST_WATER',         NULL),
    (1, 'COST_UTIL',          NULL),
    (1, 'COST_OVERHEAD',      NULL),
    (1, 'COST_AFTER_YLD',     NULL);

-- Bundle for level 0 (FG - finishing + cone packing)
INSERT INTO _txfx_capp_bundle (level, param_code, value) VALUES
    (0, 'WASTE_PCT',          2.0),
    (0, 'YIELD_PCT',          98.0),
    (0, 'LABOR_HRS',          0.005),
    (0, 'LABOR_RATE',         22000),
    (0, 'LABOR_OVERHEAD_PCT', 30),
    (0, 'IND_LABOR_PCT',      30),
    (0, 'MAT_OVERHEAD_PCT',   2),
    (0, 'MACHINE_PER_KG',     1200),
    (0, 'DEPREC_PER_KG',      300),
    (0, 'ELEC_KWH',           0.3),
    (0, 'ELEC_RATE',          1500),
    (0, 'STEAM_KG',           0),
    (0, 'STEAM_RATE',         800),
    (0, 'WATER_M3',           0),
    (0, 'WATER_RATE',         12000),
    (0, 'AIR_PER_KG',         50),
    (0, 'MAINT_PER_KG',       150),
    (0, 'FACTORY_OH',         800),
    (0, 'QC_PER_KG',          350),
    (0, 'PACK_PER_KG',        850),
    (0, 'MARGIN_PCT',         18),
    (0, 'COST_RM_TOTAL',      NULL),
    (0, 'COST_RM_LOADED',     NULL),
    (0, 'COST_ELEC',          NULL),
    (0, 'COST_LABOR',         NULL),
    (0, 'COST_LABOR_FULL',    NULL),
    (0, 'COST_CONVERSION',    NULL),
    (0, 'COST_STAGE_OUT',     NULL),
    (0, 'COST_STEAM',         NULL),
    (0, 'COST_WATER',         NULL),
    (0, 'COST_UTIL',          NULL),
    (0, 'COST_OVERHEAD',      NULL),
    (0, 'COST_AFTER_YLD',     NULL),
    (0, 'SELLING_PRICE',      NULL);

-- Insert CAPP rows (applicability) for every (product, param) in bundle.
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by
)
SELECT t.sys_id, p.id, FALSE, 'seed_000236'
  FROM _txfx_products t
  JOIN _txfx_capp_bundle b ON b.level = t.level
  JOIN mst_parameter p     ON p.param_code = b.param_code AND p.deleted_at IS NULL
 WHERE t.sys_id IS NOT NULL
   AND NOT EXISTS (
       SELECT 1 FROM cost_product_applicable_param capp
        WHERE capp.capp_product_sys_id = t.sys_id
          AND capp.capp_param_id      = p.id
 );

-- Insert CPP value rows for params with a value (skip NULL = calc-only).
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id,
    cpp_value_numeric,
    cpp_filled_by, cpp_created_by
)
SELECT t.sys_id, p.id,
       b.value,
       'seed_000236', 'seed_000236'
  FROM _txfx_products t
  JOIN _txfx_capp_bundle b ON b.level = t.level
  JOIN mst_parameter p     ON p.param_code = b.param_code AND p.deleted_at IS NULL
 WHERE t.sys_id IS NOT NULL
   AND b.value IS NOT NULL
   AND NOT EXISTS (
       SELECT 1 FROM cost_product_parameter cpp
        WHERE cpp.cpp_product_sys_id = t.sys_id
          AND cpp.cpp_param_id      = p.id
 );

COMMIT;
