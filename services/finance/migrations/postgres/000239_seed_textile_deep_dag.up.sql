-- 000239: Deep multi-stage textile fixture (S8e-fix revamp).
--
-- Replaces the shallow ~30-product / 1-seq fixture seeded by 000236 with a
-- DAG that mirrors how a real Indonesian polyester+textile mill actually
-- processes goods:
--
--   Level 11  Raw chips         (TXFX_C1_*)  - made from ITEM RMs only
--   Level 10  Blended chips     (TXFX_C2_*)  - MERGE of raw + masterbatch
--   Level  9  Dried chips       (TXFX_C3_*)  - multi-seq (4 stages)
--   Level  8  Undrawn POY       (TXFX_PU_*)
--   Level  7  Drawn POY         (TXFX_P_*)   - SPLIT (1 undrawn -> 2-4 deniers)
--   Level  6  Drawn pre-set     (TXFX_DP_*)
--   Level  5  Drawn-HT          (TXFX_DH_*)
--   Level  4  Drawn-LT          (TXFX_DL_*)  - SPLIT from preset (HT + LT)
--   Level  3  DTY               (TXFX_DTY_*) - MERGE of HT + LT
--   Level  2  Greige fabric     (TXFX_G_*)   - MERGE of multiple DTYs
--   Level  1  Dyed fabric       (TXFX_DY_*)  - multi-seq (4 stages)
--   Level  0  FG                (TXFX_FG_*)  - multi-seq (4 stages)
--
-- Total: ~55 products spanning 12 logical levels with linear + split + merge
-- patterns. The deepest FG walks 11 PRODUCT-RM hops back to a raw chip.
--
-- Routes are MULTI-STAGE per product (route_head -> N route_seq -> route_rm):
--   * Chip dried: 4 seqs (Polymerize -> Cool -> Crystallize -> Dry)
--   * POY drawn: 2 seqs (Spin -> Inspect)
--   * DTY:       3 seqs (Texture -> Intermingle -> Wind)
--   * Greige:    2 seqs (Knit -> Inspect)
--   * Dyed:      4 seqs (Scour -> Dye -> Soap -> Dry)
--   * FG:        4 seqs (Heat-Set -> Inspect -> Cut -> Pack)
--   * Others (raw chip, blended chip, undrawn POY, preset/HT/LT): 1 seq
--
-- PRODUCT-RMs are placed on the first seq of each route. ITEM-RMs (chemicals)
-- are placed on the seq where they are actually consumed -- e.g. dyestuff
-- lives on the "Dye" seq, caustic on "Scour", masterbatch on the chip blend.
--
-- ITEM RM REMAPPING: this seed does NOT insert any rows into cst_rm_cost.
-- Instead it references existing real rm_codes already in the table (sourced
-- from Oracle sync). Mapping:
--     PIGMENT/DYE BLACK  -> 202503831 (PALIOGEN RED K 3127 B, cost ~199)
--     PIGMENT NAVY       -> 202007161 (BLUE MGTS-5054, cost ~60)
--     PIGMENT RED        -> 202007193 (RED MGTS 3059, cost ~59)
--     PIGMENT YELLOW     -> 202007205 (YELLOW MGTP-2159 BASF, cost ~107)
--     PIGMENT ORANGE     -> 202007179 (ORANGE MGTS-3072, cost ~63)
--     PIGMENT VIOLET     -> 202007203 (VIOLET MGTS-5062, cost ~96)
--     PIGMENT GREY/BROWN -> 202007166 (BROWN MGTS-6061, cost ~40)
--     DYESTUFF GENERAL   -> 202108613 (DYE0000045, cost ~73)
--     DYESTUFF AUX1      -> 202411819 (DYE0000055, cost ~38)
--     MASTERBATCH        -> 202303677 (MBC0000291, cost ~66)
-- Polymer feedstock (PTA / MEG / Spin Oil / TiO2) and process chemicals
-- (NaOH / Acetic Acid / Cone) are intentionally NOT mapped: the upstream chip
-- products are seeded with ONLY PRODUCT-RM dependencies on masterbatch (so the
-- engine still gets a non-zero base cost from the existing real masterbatch
-- rm_code). The calc engine accepts ITEM rows missing in cst_rm_cost as zero
-- contribution; we omit those rather than fabricate codes.
--
-- Idempotency: every INSERT uses NOT EXISTS guards. Replayable.

BEGIN;

-- =============================================================================
-- 0. Per-run lookup table for product code -> sys_id resolution.
-- =============================================================================
CREATE TEMP TABLE _txfx2_products (
    code              VARCHAR(20) PRIMARY KEY,
    name              TEXT NOT NULL,
    product_type_code VARCHAR(5) NOT NULL,
    level             INT NOT NULL,
    sys_id            BIGINT
);

INSERT INTO _txfx2_products (code, name, product_type_code, level) VALUES
    -- Level 11: raw chips (RM-only)
    ('TXFX_C1_SD',         'Chip Semi-Dull (raw, ex-polymerizer)', 'INTER', 11),
    ('TXFX_C1_BR',         'Chip Bright (raw, ex-polymerizer)',    'INTER', 11),
    ('TXFX_C1_FD',         'Chip Full-Dull (raw, ex-polymerizer)', 'INTER', 11),
    ('TXFX_C1_MB_BLK',     'Black Masterbatch chip',               'INTER', 11),
    ('TXFX_C1_MB_NVY',     'Navy Masterbatch chip',                'INTER', 11),
    -- Level 10: blended chips (raw + masterbatch)
    ('TXFX_C2_SD',         'Chip Semi-Dull blended w/ Black MB',   'INTER', 10),
    ('TXFX_C2_BR',         'Chip Bright blended w/ Navy MB',       'INTER', 10),
    ('TXFX_C2_FD',         'Chip Full-Dull (natural, no MB)',      'INTER', 10),
    -- Level 9: dried/crystallized chip (multi-stage route)
    ('TXFX_C3_SD',         'Chip SD dried+crystallized',           'INTER', 9),
    ('TXFX_C3_BR',         'Chip BR dried+crystallized',           'INTER', 9),
    ('TXFX_C3_FD',         'Chip FD dried+crystallized',           'INTER', 9),
    -- Level 8: undrawn POY
    ('TXFX_PU_SD',         'Undrawn POY Semi-Dull',                'POY',   8),
    ('TXFX_PU_BR',         'Undrawn POY Bright',                   'POY',   8),
    ('TXFX_PU_FD',         'Undrawn POY Full-Dull',                'POY',   8),
    -- Level 7: drawn POY (SPLIT: undrawn -> multi denier)
    ('TXFX_P_075SD',       'POY 75D/36F Semi-Dull',                'POY',   7),
    ('TXFX_P_150SD',       'POY 150D/48F Semi-Dull',               'POY',   7),
    ('TXFX_P_300SD',       'POY 300D/96F Semi-Dull',               'POY',   7),
    ('TXFX_P_450SD',       'POY 450D/144F Semi-Dull',              'POY',   7),
    ('TXFX_P_075BR',       'POY 75D/36F Bright',                   'POY',   7),
    ('TXFX_P_150BR',       'POY 150D/48F Bright',                  'POY',   7),
    ('TXFX_P_300FD',       'POY 300D/96F Full-Dull',               'POY',   7),
    -- Level 6: drawn pre-set
    ('TXFX_DP_075SD',      'Drawn Pre-set 75D SD',                 'INTER', 6),
    ('TXFX_DP_150SD',      'Drawn Pre-set 150D SD',                'INTER', 6),
    ('TXFX_DP_300SD',      'Drawn Pre-set 300D SD',                'INTER', 6),
    ('TXFX_DP_075BR',      'Drawn Pre-set 75D BR',                 'INTER', 6),
    -- Level 5: drawn HT (high-twist)
    ('TXFX_DH_075SD',      'Drawn-HT 75D SD',                      'INTER', 5),
    ('TXFX_DH_150SD',      'Drawn-HT 150D SD',                     'INTER', 5),
    ('TXFX_DH_300SD',      'Drawn-HT 300D SD',                     'INTER', 5),
    -- Level 4: drawn LT (low-twist) -- SPLIT from preset
    ('TXFX_DL_075SD',      'Drawn-LT 75D SD',                      'INTER', 4),
    ('TXFX_DL_150SD',      'Drawn-LT 150D SD',                     'INTER', 4),
    ('TXFX_DL_300SD',      'Drawn-LT 300D SD',                     'INTER', 4),
    -- Level 3: DTY (MERGE of HT + LT)
    ('TXFX_DTY_075SD_N',   'DTY 75D SD Non-Intermingled',          'DTY',   3),
    ('TXFX_DTY_075SD_I',   'DTY 75D SD Intermingled',              'DTY',   3),
    ('TXFX_DTY_150SD_N',   'DTY 150D SD Non-Intermingled',         'DTY',   3),
    ('TXFX_DTY_150SD_I',   'DTY 150D SD Intermingled',             'DTY',   3),
    ('TXFX_DTY_300SD',     'DTY 300D SD',                          'DTY',   3),
    ('TXFX_DTY_075BR',     'DTY 75D Bright',                       'DTY',   3),
    -- Level 2: greige fabric (MERGE)
    ('TXFX_G_JRS_180',     'Greige Jersey 180gsm',                 'INTER', 2),
    ('TXFX_G_JRS_240',     'Greige Jersey 240gsm',                 'INTER', 2),
    ('TXFX_G_RIB_220',     'Greige Rib 220gsm',                    'INTER', 2),
    ('TXFX_G_TRC_120',     'Greige Tricot 120gsm',                 'INTER', 2),
    ('TXFX_G_SPC_280',     'Greige Spacer 280gsm',                 'INTER', 2),
    ('TXFX_G_PIQ_200',     'Greige Pique 200gsm',                  'INTER', 2),
    -- Level 1: dyed fabric (multi-stage route)
    ('TXFX_DY_JRS_BLK',    'Dyed Jersey 180gsm Black',             'INTER', 1),
    ('TXFX_DY_JRS_NVY',    'Dyed Jersey 180gsm Navy',              'INTER', 1),
    ('TXFX_DY_JRS_RED',    'Dyed Jersey 240gsm Red',               'INTER', 1),
    ('TXFX_DY_TRC_RED',    'Dyed Tricot 120gsm Red',               'INTER', 1),
    ('TXFX_DY_RIB_WHT',    'Dyed Rib 220gsm Optical White',        'INTER', 1),
    ('TXFX_DY_SPC_GRY',    'Dyed Spacer 280gsm Grey',              'INTER', 1),
    ('TXFX_DY_PIQ_BLK',    'Dyed Pique 200gsm Black',              'INTER', 1),
    -- Level 0: FG (multi-stage finishing + packing)
    ('TXFX_FG_JRS_BLK',    'FG Jersey Black 180gsm',               'FG',    0),
    ('TXFX_FG_JRS_NVY',    'FG Jersey Navy 180gsm',                'FG',    0),
    ('TXFX_FG_JRS_RED',    'FG Jersey Red 240gsm',                 'FG',    0),
    ('TXFX_FG_TRC_RED',    'FG Tricot Red 120gsm',                 'FG',    0),
    ('TXFX_FG_RIB_WHT',    'FG Rib Optical White 220gsm',          'FG',    0),
    ('TXFX_FG_SPC_GRY',    'FG Spacer Grey 280gsm',                'FG',    0),
    ('TXFX_FG_PIQ_BLK',    'FG Pique Black 200gsm',                'FG',    0),
    ('TXFX_FG_DTY075I_CN', 'FG DTY 75D SD IM cone yarn',           'FG',    0);

-- =============================================================================
-- 1. Insert cost_product_master rows.
-- =============================================================================
INSERT INTO cost_product_master (
    cpm_product_code, cpm_product_name, cpm_product_type_id, cpm_grade_code,
    cpm_description, cpm_is_active, cpm_created_by, cpm_updated_by
)
SELECT v.code, v.name, ct.cpt_type_id, 'AX',
       'S8e-fix textile deep DAG fixture', TRUE, 'seed_000239', 'seed_000239'
  FROM _txfx2_products v
  JOIN cost_product_type ct ON ct.cpt_type_code = v.product_type_code
 WHERE NOT EXISTS (
       SELECT 1 FROM cost_product_master cpm WHERE cpm.cpm_product_code = v.code
 );

UPDATE _txfx2_products t
   SET sys_id = cpm.cpm_product_sys_id
  FROM cost_product_master cpm
 WHERE cpm.cpm_product_code = t.code;

-- =============================================================================
-- 2. Route definitions: head (1 per product) + N seqs (stages) + rms per seq.
--
-- _txfx2_route_seqs: (product_code, seq_no, seq_name) -- ordered stages within
-- the product's route. _txfx2_route_rms: (product_code, seq_no, rm_type,
-- rm_ref, ratio) -- which RM is consumed at which stage.
-- =============================================================================
CREATE TEMP TABLE _txfx2_route_seqs (
    code     VARCHAR(20) NOT NULL,
    seq_no   INT NOT NULL,
    seq_name VARCHAR(200) NOT NULL,
    PRIMARY KEY (code, seq_no)
);

CREATE TEMP TABLE _txfx2_route_rms (
    code      VARCHAR(20) NOT NULL,
    seq_no    INT NOT NULL,
    rm_type   VARCHAR(20) NOT NULL,   -- 'PRODUCT' or 'ITEM'
    rm_ref    VARCHAR(30) NOT NULL,   -- product code or actual rm_code
    ratio     NUMERIC NOT NULL
);

-- ----- Level 11 (raw chips): 1 seq, no upstream products. NO RMs seeded (the
--       calc engine treats route_rm-less seqs as zero RM contribution; chip
--       costs propagate down from masterbatch through the C2 blend layer).
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_C1_SD',       1, 'Polymerize SD'),
    ('TXFX_C1_BR',       1, 'Polymerize BR'),
    ('TXFX_C1_FD',       1, 'Polymerize FD'),
    ('TXFX_C1_MB_BLK',   1, 'Compound Black masterbatch'),
    ('TXFX_C1_MB_NVY',   1, 'Compound Navy masterbatch');

-- Masterbatch chips have one real ITEM RM each (the pigment).
INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_C1_MB_BLK',   1, 'ITEM', '202503831', 0.030),  -- pigment black-ish
    ('TXFX_C1_MB_NVY',   1, 'ITEM', '202007161', 0.030);  -- BLUE MGTS-5054

-- ----- Level 10 (blended chips): MERGE = raw chip + masterbatch.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_C2_SD',       1, 'Blend SD + Black MB'),
    ('TXFX_C2_BR',       1, 'Blend BR + Navy MB'),
    ('TXFX_C2_FD',       1, 'Blend FD (natural)');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_C2_SD',       1, 'PRODUCT', 'TXFX_C1_SD',     0.970),
    ('TXFX_C2_SD',       1, 'PRODUCT', 'TXFX_C1_MB_BLK', 0.030),
    ('TXFX_C2_BR',       1, 'PRODUCT', 'TXFX_C1_BR',     0.970),
    ('TXFX_C2_BR',       1, 'PRODUCT', 'TXFX_C1_MB_NVY', 0.030),
    ('TXFX_C2_FD',       1, 'PRODUCT', 'TXFX_C1_FD',     1.000),
    -- A real-world masterbatch additive too, sometimes used in FD:
    ('TXFX_C2_FD',       1, 'ITEM',    '202303677',      0.005);  -- MBC0000291

-- ----- Level 9 (dried/crystallized chip): 4 seqs (Polymerize -> Cool ->
--       Crystallize -> Dry). PRODUCT-RM on seq 1.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_C3_SD',       1, 'Receive blended SD chip'),
    ('TXFX_C3_SD',       2, 'Cool'),
    ('TXFX_C3_SD',       3, 'Crystallize'),
    ('TXFX_C3_SD',       4, 'Dry'),
    ('TXFX_C3_BR',       1, 'Receive blended BR chip'),
    ('TXFX_C3_BR',       2, 'Cool'),
    ('TXFX_C3_BR',       3, 'Crystallize'),
    ('TXFX_C3_BR',       4, 'Dry'),
    ('TXFX_C3_FD',       1, 'Receive blended FD chip'),
    ('TXFX_C3_FD',       2, 'Cool'),
    ('TXFX_C3_FD',       3, 'Crystallize'),
    ('TXFX_C3_FD',       4, 'Dry');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_C3_SD',       1, 'PRODUCT', 'TXFX_C2_SD', 1.005),
    ('TXFX_C3_BR',       1, 'PRODUCT', 'TXFX_C2_BR', 1.005),
    ('TXFX_C3_FD',       1, 'PRODUCT', 'TXFX_C2_FD', 1.005);

-- ----- Level 8 (undrawn POY): 1 seq.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_PU_SD', 1, 'Spin undrawn POY SD'),
    ('TXFX_PU_BR', 1, 'Spin undrawn POY BR'),
    ('TXFX_PU_FD', 1, 'Spin undrawn POY FD');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_PU_SD', 1, 'PRODUCT', 'TXFX_C3_SD', 1.018),
    ('TXFX_PU_BR', 1, 'PRODUCT', 'TXFX_C3_BR', 1.018),
    ('TXFX_PU_FD', 1, 'PRODUCT', 'TXFX_C3_FD', 1.018);

-- ----- Level 7 (drawn POY): 2 seqs (Draw -> Inspect). SPLIT pattern: PU_SD
--       feeds 4 deniers, PU_BR feeds 2, PU_FD feeds 1.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_P_075SD', 1, 'Draw 75D from undrawn SD'),  ('TXFX_P_075SD', 2, 'Inspect+Wind 75D'),
    ('TXFX_P_150SD', 1, 'Draw 150D from undrawn SD'), ('TXFX_P_150SD', 2, 'Inspect+Wind 150D'),
    ('TXFX_P_300SD', 1, 'Draw 300D from undrawn SD'), ('TXFX_P_300SD', 2, 'Inspect+Wind 300D'),
    ('TXFX_P_450SD', 1, 'Draw 450D from undrawn SD'), ('TXFX_P_450SD', 2, 'Inspect+Wind 450D'),
    ('TXFX_P_075BR', 1, 'Draw 75D from undrawn BR'),  ('TXFX_P_075BR', 2, 'Inspect+Wind 75D BR'),
    ('TXFX_P_150BR', 1, 'Draw 150D from undrawn BR'), ('TXFX_P_150BR', 2, 'Inspect+Wind 150D BR'),
    ('TXFX_P_300FD', 1, 'Draw 300D from undrawn FD'), ('TXFX_P_300FD', 2, 'Inspect+Wind 300D FD');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_P_075SD', 1, 'PRODUCT', 'TXFX_PU_SD', 1.020),
    ('TXFX_P_150SD', 1, 'PRODUCT', 'TXFX_PU_SD', 1.020),
    ('TXFX_P_300SD', 1, 'PRODUCT', 'TXFX_PU_SD', 1.020),
    ('TXFX_P_450SD', 1, 'PRODUCT', 'TXFX_PU_SD', 1.020),
    ('TXFX_P_075BR', 1, 'PRODUCT', 'TXFX_PU_BR', 1.020),
    ('TXFX_P_150BR', 1, 'PRODUCT', 'TXFX_PU_BR', 1.020),
    ('TXFX_P_300FD', 1, 'PRODUCT', 'TXFX_PU_FD', 1.020);

-- ----- Level 6 (drawn pre-set): 1 seq.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_DP_075SD', 1, 'Pre-set 75D SD'),
    ('TXFX_DP_150SD', 1, 'Pre-set 150D SD'),
    ('TXFX_DP_300SD', 1, 'Pre-set 300D SD'),
    ('TXFX_DP_075BR', 1, 'Pre-set 75D BR');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_DP_075SD', 1, 'PRODUCT', 'TXFX_P_075SD', 1.005),
    ('TXFX_DP_150SD', 1, 'PRODUCT', 'TXFX_P_150SD', 1.005),
    ('TXFX_DP_300SD', 1, 'PRODUCT', 'TXFX_P_300SD', 1.005),
    ('TXFX_DP_075BR', 1, 'PRODUCT', 'TXFX_P_075BR', 1.005);

-- ----- Level 5 (drawn-HT): 1 seq, from preset.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_DH_075SD', 1, 'High-twist 75D SD'),
    ('TXFX_DH_150SD', 1, 'High-twist 150D SD'),
    ('TXFX_DH_300SD', 1, 'High-twist 300D SD');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_DH_075SD', 1, 'PRODUCT', 'TXFX_DP_075SD', 0.520),
    ('TXFX_DH_150SD', 1, 'PRODUCT', 'TXFX_DP_150SD', 0.520),
    ('TXFX_DH_300SD', 1, 'PRODUCT', 'TXFX_DP_300SD', 0.520);

-- ----- Level 4 (drawn-LT): 1 seq, from preset (SPLIT: preset -> HT + LT).
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_DL_075SD', 1, 'Low-twist 75D SD'),
    ('TXFX_DL_150SD', 1, 'Low-twist 150D SD'),
    ('TXFX_DL_300SD', 1, 'Low-twist 300D SD');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_DL_075SD', 1, 'PRODUCT', 'TXFX_DP_075SD', 0.500),
    ('TXFX_DL_150SD', 1, 'PRODUCT', 'TXFX_DP_150SD', 0.500),
    ('TXFX_DL_300SD', 1, 'PRODUCT', 'TXFX_DP_300SD', 0.500);

-- ----- Level 3 (DTY): 3 seqs (Texture -> Intermingle -> Wind). MERGE = HT+LT.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_DTY_075SD_N', 1, 'Texture 75D SD NIM'), ('TXFX_DTY_075SD_N', 2, 'Light intermingle'), ('TXFX_DTY_075SD_N', 3, 'Wind 75D NIM'),
    ('TXFX_DTY_075SD_I', 1, 'Texture 75D SD IM'),  ('TXFX_DTY_075SD_I', 2, 'Heavy intermingle'), ('TXFX_DTY_075SD_I', 3, 'Wind 75D IM'),
    ('TXFX_DTY_150SD_N', 1, 'Texture 150D SD NIM'),('TXFX_DTY_150SD_N', 2, 'Light intermingle'), ('TXFX_DTY_150SD_N', 3, 'Wind 150D NIM'),
    ('TXFX_DTY_150SD_I', 1, 'Texture 150D SD IM'), ('TXFX_DTY_150SD_I', 2, 'Heavy intermingle'), ('TXFX_DTY_150SD_I', 3, 'Wind 150D IM'),
    ('TXFX_DTY_300SD',   1, 'Texture 300D SD'),    ('TXFX_DTY_300SD',   2, 'Intermingle'),       ('TXFX_DTY_300SD',   3, 'Wind 300D'),
    ('TXFX_DTY_075BR',   1, 'Texture 75D BR'),     ('TXFX_DTY_075BR',   2, 'Intermingle'),       ('TXFX_DTY_075BR',   3, 'Wind 75D BR');

INSERT INTO _txfx2_route_rms VALUES
    -- 075SD_N: 60% HT + 40% LT (lighter intermingle = HT-heavy)
    ('TXFX_DTY_075SD_N', 1, 'PRODUCT', 'TXFX_DH_075SD', 0.610),
    ('TXFX_DTY_075SD_N', 1, 'PRODUCT', 'TXFX_DL_075SD', 0.410),
    -- 075SD_I: 50%/50% (heavy intermingle uses more LT)
    ('TXFX_DTY_075SD_I', 1, 'PRODUCT', 'TXFX_DH_075SD', 0.510),
    ('TXFX_DTY_075SD_I', 1, 'PRODUCT', 'TXFX_DL_075SD', 0.510),
    ('TXFX_DTY_150SD_N', 1, 'PRODUCT', 'TXFX_DH_150SD', 0.620),
    ('TXFX_DTY_150SD_N', 1, 'PRODUCT', 'TXFX_DL_150SD', 0.400),
    ('TXFX_DTY_150SD_I', 1, 'PRODUCT', 'TXFX_DH_150SD', 0.520),
    ('TXFX_DTY_150SD_I', 1, 'PRODUCT', 'TXFX_DL_150SD', 0.510),
    ('TXFX_DTY_300SD',   1, 'PRODUCT', 'TXFX_DH_300SD', 0.560),
    ('TXFX_DTY_300SD',   1, 'PRODUCT', 'TXFX_DL_300SD', 0.470),
    ('TXFX_DTY_075BR',   1, 'PRODUCT', 'TXFX_DP_075BR', 1.020);

-- ----- Level 2 (greige): 2 seqs (Knit -> Inspect). MERGE of multiple DTYs.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_G_JRS_180', 1, 'Knit Jersey 180gsm'),  ('TXFX_G_JRS_180', 2, 'Inspect Jersey 180'),
    ('TXFX_G_JRS_240', 1, 'Knit Jersey 240gsm'),  ('TXFX_G_JRS_240', 2, 'Inspect Jersey 240'),
    ('TXFX_G_RIB_220', 1, 'Knit Rib 220gsm'),     ('TXFX_G_RIB_220', 2, 'Inspect Rib 220'),
    ('TXFX_G_TRC_120', 1, 'Knit Tricot 120gsm'),  ('TXFX_G_TRC_120', 2, 'Inspect Tricot 120'),
    ('TXFX_G_SPC_280', 1, 'Knit Spacer 280gsm'),  ('TXFX_G_SPC_280', 2, 'Inspect Spacer 280'),
    ('TXFX_G_PIQ_200', 1, 'Knit Pique 200gsm'),   ('TXFX_G_PIQ_200', 2, 'Inspect Pique 200');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_G_JRS_180', 1, 'PRODUCT', 'TXFX_DTY_150SD_N', 0.650),
    ('TXFX_G_JRS_180', 1, 'PRODUCT', 'TXFX_DTY_075SD_N', 0.370),
    ('TXFX_G_JRS_240', 1, 'PRODUCT', 'TXFX_DTY_150SD_I', 0.620),
    ('TXFX_G_JRS_240', 1, 'PRODUCT', 'TXFX_DTY_300SD',   0.410),
    ('TXFX_G_RIB_220', 1, 'PRODUCT', 'TXFX_DTY_150SD_I', 0.700),
    ('TXFX_G_RIB_220', 1, 'PRODUCT', 'TXFX_DTY_300SD',   0.320),
    ('TXFX_G_TRC_120', 1, 'PRODUCT', 'TXFX_DTY_075SD_I', 1.030),
    ('TXFX_G_SPC_280', 1, 'PRODUCT', 'TXFX_DTY_150SD_N', 0.620),
    ('TXFX_G_SPC_280', 1, 'PRODUCT', 'TXFX_DTY_075BR',   0.400),
    ('TXFX_G_PIQ_200', 1, 'PRODUCT', 'TXFX_DTY_150SD_N', 0.580),
    ('TXFX_G_PIQ_200', 1, 'PRODUCT', 'TXFX_DTY_075SD_I', 0.430);

-- ----- Level 1 (dyed): 4 seqs (Scour -> Dye -> Soap -> Dry). PRODUCT-RM on
--       Scour seq; dyestuff on Dye seq; soap/aux on Soap seq.
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_DY_JRS_BLK', 1, 'Scour Jersey'), ('TXFX_DY_JRS_BLK', 2, 'Dye Black'),  ('TXFX_DY_JRS_BLK', 3, 'Soap'), ('TXFX_DY_JRS_BLK', 4, 'Dry'),
    ('TXFX_DY_JRS_NVY', 1, 'Scour Jersey'), ('TXFX_DY_JRS_NVY', 2, 'Dye Navy'),   ('TXFX_DY_JRS_NVY', 3, 'Soap'), ('TXFX_DY_JRS_NVY', 4, 'Dry'),
    ('TXFX_DY_JRS_RED', 1, 'Scour Jersey'), ('TXFX_DY_JRS_RED', 2, 'Dye Red'),    ('TXFX_DY_JRS_RED', 3, 'Soap'), ('TXFX_DY_JRS_RED', 4, 'Dry'),
    ('TXFX_DY_TRC_RED', 1, 'Scour Tricot'), ('TXFX_DY_TRC_RED', 2, 'Dye Red'),    ('TXFX_DY_TRC_RED', 3, 'Soap'), ('TXFX_DY_TRC_RED', 4, 'Dry'),
    ('TXFX_DY_RIB_WHT', 1, 'Scour Rib'),    ('TXFX_DY_RIB_WHT', 2, 'Bleach+OB'),  ('TXFX_DY_RIB_WHT', 3, 'Soap'), ('TXFX_DY_RIB_WHT', 4, 'Dry'),
    ('TXFX_DY_SPC_GRY', 1, 'Scour Spacer'), ('TXFX_DY_SPC_GRY', 2, 'Dye Grey'),   ('TXFX_DY_SPC_GRY', 3, 'Soap'), ('TXFX_DY_SPC_GRY', 4, 'Dry'),
    ('TXFX_DY_PIQ_BLK', 1, 'Scour Pique'),  ('TXFX_DY_PIQ_BLK', 2, 'Dye Black'),  ('TXFX_DY_PIQ_BLK', 3, 'Soap'), ('TXFX_DY_PIQ_BLK', 4, 'Dry');

-- Dyed RMs: PRODUCT (greige) on seq 1; dyestuff on seq 2; soap on seq 3.
INSERT INTO _txfx2_route_rms VALUES
    -- BLK Jersey
    ('TXFX_DY_JRS_BLK', 1, 'PRODUCT', 'TXFX_G_JRS_180', 1.050),
    ('TXFX_DY_JRS_BLK', 2, 'ITEM',    '202503831',      0.040),  -- "black" dye stand-in (PALIOGEN RED K)
    ('TXFX_DY_JRS_BLK', 3, 'ITEM',    '202108613',      0.010),  -- DYE0000045
    -- NVY Jersey
    ('TXFX_DY_JRS_NVY', 1, 'PRODUCT', 'TXFX_G_JRS_180', 1.050),
    ('TXFX_DY_JRS_NVY', 2, 'ITEM',    '202007161',      0.035),  -- BLUE MGTS-5054
    ('TXFX_DY_JRS_NVY', 3, 'ITEM',    '202411819',      0.010),  -- DYE0000055
    -- RED Jersey (heavy)
    ('TXFX_DY_JRS_RED', 1, 'PRODUCT', 'TXFX_G_JRS_240', 1.050),
    ('TXFX_DY_JRS_RED', 2, 'ITEM',    '202007193',      0.045),  -- RED MGTS 3059
    ('TXFX_DY_JRS_RED', 3, 'ITEM',    '202108613',      0.010),
    -- RED Tricot
    ('TXFX_DY_TRC_RED', 1, 'PRODUCT', 'TXFX_G_TRC_120', 1.050),
    ('TXFX_DY_TRC_RED', 2, 'ITEM',    '202007193',      0.030),
    ('TXFX_DY_TRC_RED', 3, 'ITEM',    '202411819',      0.008),
    -- WHT Rib
    ('TXFX_DY_RIB_WHT', 1, 'PRODUCT', 'TXFX_G_RIB_220', 1.045),
    ('TXFX_DY_RIB_WHT', 2, 'ITEM',    '202007205',      0.015),  -- YELLOW MGTP-2159 (OB stand-in)
    ('TXFX_DY_RIB_WHT', 3, 'ITEM',    '202108613',      0.008),
    -- GRY Spacer
    ('TXFX_DY_SPC_GRY', 1, 'PRODUCT', 'TXFX_G_SPC_280', 1.045),
    ('TXFX_DY_SPC_GRY', 2, 'ITEM',    '202007166',      0.025),  -- BROWN MGTS-6061 (grey stand-in)
    ('TXFX_DY_SPC_GRY', 3, 'ITEM',    '202411819',      0.010),
    -- BLK Pique
    ('TXFX_DY_PIQ_BLK', 1, 'PRODUCT', 'TXFX_G_PIQ_200', 1.050),
    ('TXFX_DY_PIQ_BLK', 2, 'ITEM',    '202503831',      0.040),
    ('TXFX_DY_PIQ_BLK', 3, 'ITEM',    '202108613',      0.010);

-- ----- Level 0 (FG): 4 seqs (Heat-Set -> Inspect -> Cut -> Pack).
INSERT INTO _txfx2_route_seqs VALUES
    ('TXFX_FG_JRS_BLK', 1, 'Heat-Set Jersey'), ('TXFX_FG_JRS_BLK', 2, 'Inspect'), ('TXFX_FG_JRS_BLK', 3, 'Cut'), ('TXFX_FG_JRS_BLK', 4, 'Pack'),
    ('TXFX_FG_JRS_NVY', 1, 'Heat-Set Jersey'), ('TXFX_FG_JRS_NVY', 2, 'Inspect'), ('TXFX_FG_JRS_NVY', 3, 'Cut'), ('TXFX_FG_JRS_NVY', 4, 'Pack'),
    ('TXFX_FG_JRS_RED', 1, 'Heat-Set Jersey'), ('TXFX_FG_JRS_RED', 2, 'Inspect'), ('TXFX_FG_JRS_RED', 3, 'Cut'), ('TXFX_FG_JRS_RED', 4, 'Pack'),
    ('TXFX_FG_TRC_RED', 1, 'Heat-Set Tricot'), ('TXFX_FG_TRC_RED', 2, 'Inspect'), ('TXFX_FG_TRC_RED', 3, 'Cut'), ('TXFX_FG_TRC_RED', 4, 'Pack'),
    ('TXFX_FG_RIB_WHT', 1, 'Heat-Set Rib'),    ('TXFX_FG_RIB_WHT', 2, 'Inspect'), ('TXFX_FG_RIB_WHT', 3, 'Cut'), ('TXFX_FG_RIB_WHT', 4, 'Pack'),
    ('TXFX_FG_SPC_GRY', 1, 'Heat-Set Spacer'), ('TXFX_FG_SPC_GRY', 2, 'Inspect'), ('TXFX_FG_SPC_GRY', 3, 'Cut'), ('TXFX_FG_SPC_GRY', 4, 'Pack'),
    ('TXFX_FG_PIQ_BLK', 1, 'Heat-Set Pique'),  ('TXFX_FG_PIQ_BLK', 2, 'Inspect'), ('TXFX_FG_PIQ_BLK', 3, 'Cut'), ('TXFX_FG_PIQ_BLK', 4, 'Pack'),
    ('TXFX_FG_DTY075I_CN', 1, 'Cone-wind QC'), ('TXFX_FG_DTY075I_CN', 2, 'Inspect cone'), ('TXFX_FG_DTY075I_CN', 3, 'Label'), ('TXFX_FG_DTY075I_CN', 4, 'Pack carton');

INSERT INTO _txfx2_route_rms VALUES
    ('TXFX_FG_JRS_BLK',    1, 'PRODUCT', 'TXFX_DY_JRS_BLK',    1.020),
    ('TXFX_FG_JRS_NVY',    1, 'PRODUCT', 'TXFX_DY_JRS_NVY',    1.020),
    ('TXFX_FG_JRS_RED',    1, 'PRODUCT', 'TXFX_DY_JRS_RED',    1.020),
    ('TXFX_FG_TRC_RED',    1, 'PRODUCT', 'TXFX_DY_TRC_RED',    1.020),
    ('TXFX_FG_RIB_WHT',    1, 'PRODUCT', 'TXFX_DY_RIB_WHT',    1.020),
    ('TXFX_FG_SPC_GRY',    1, 'PRODUCT', 'TXFX_DY_SPC_GRY',    1.020),
    ('TXFX_FG_PIQ_BLK',    1, 'PRODUCT', 'TXFX_DY_PIQ_BLK',    1.020),
    -- FG cone yarn -- ITEM cone tube on Pack seq (seq 4)
    ('TXFX_FG_DTY075I_CN', 1, 'PRODUCT', 'TXFX_DTY_075SD_I',   1.005),
    ('TXFX_FG_DTY075I_CN', 4, 'ITEM',    '202303677',          0.020);  -- MBC0000291 stand-in for cone packing additive

-- =============================================================================
-- 3. Build cost_route_head + cost_route_seq + cost_route_rm via PL/pgSQL loop.
-- =============================================================================
DO $$
DECLARE
    p              RECORD;
    s              RECORD;
    rm             RECORD;
    v_head_id      BIGINT;
    v_seq_id       BIGINT;
    v_product_id   BIGINT;
    v_rm_product_id BIGINT;
BEGIN
    FOR p IN SELECT code, sys_id FROM _txfx2_products WHERE sys_id IS NOT NULL ORDER BY level DESC, code
    LOOP
        v_product_id := p.sys_id;

        -- Skip if a non-LOCKED route head already exists (defensive replayability).
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
            'Deep DAG fixture for ' || p.code,
            'seed_000239', 'seed_000239'
        ) RETURNING crh_head_id INTO v_head_id;

        -- One row per (product, seq_no) -> one cost_route_seq.
        FOR s IN
            SELECT seq_no, seq_name FROM _txfx2_route_seqs
             WHERE code = p.code ORDER BY seq_no
        LOOP
            INSERT INTO cost_route_seq (
                crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
                crs_route_name, crs_created_by, crs_updated_by
            ) VALUES (
                v_head_id, v_product_id, s.seq_no, s.seq_no,
                s.seq_name, 'seed_000239', 'seed_000239'
            ) RETURNING crs_seq_id INTO v_seq_id;

            -- RMs for this specific seq.
            FOR rm IN
                SELECT rm_type, rm_ref, ratio FROM _txfx2_route_rms
                 WHERE code = p.code AND seq_no = s.seq_no
            LOOP
                v_rm_product_id := NULL;
                IF rm.rm_type = 'PRODUCT' THEN
                    SELECT sys_id INTO v_rm_product_id
                      FROM _txfx2_products WHERE code = rm.rm_ref;
                    IF v_rm_product_id IS NULL THEN
                        RAISE EXCEPTION 'unresolved PRODUCT rm: % for %', rm.rm_ref, p.code;
                    END IF;
                    INSERT INTO cost_route_rm (
                        crm_seq_id, crm_parent_product_sys_id,
                        crm_rm_product_sys_id, crm_rm_type, crm_route_rm_name,
                        crm_route_rm_ratio,
                        crm_created_by, crm_updated_by
                    ) VALUES (
                        v_seq_id, v_product_id,
                        v_rm_product_id, 'PRODUCT', rm.rm_ref,
                        rm.ratio,
                        'seed_000239', 'seed_000239'
                    );
                ELSE
                    INSERT INTO cost_route_rm (
                        crm_seq_id, crm_parent_product_sys_id,
                        crm_rm_item_code, crm_rm_type, crm_route_rm_name, crm_route_rm_item_code,
                        crm_route_rm_ratio,
                        crm_created_by, crm_updated_by
                    ) VALUES (
                        v_seq_id, v_product_id,
                        rm.rm_ref, 'ITEM', rm.rm_ref, rm.rm_ref,
                        rm.ratio,
                        'seed_000239', 'seed_000239'
                    );
                END IF;
            END LOOP;
        END LOOP;
    END LOOP;
END $$;

-- =============================================================================
-- 4. CAPP rows + cost_product_parameter values per level bundle.
--    Re-uses the same 6 bundles as 000236 but applied across the deeper level
--    set (0..11). Levels 11/10/9 share the "chip" economics, 8/7/6/5/4 share
--    yarn economics, etc. Bundle map:
--        L11..L9   -> chip   (matches old L5 bundle)
--        L8..L7    -> POY    (old L4)
--        L6..L4    -> drawn intermediate (reuse POY bundle, similar economics)
--        L3        -> DTY    (old L3)
--        L2        -> greige (old L2)
--        L1        -> dyed   (old L1)
--        L0        -> FG     (old L0)
-- =============================================================================
CREATE TEMP TABLE _txfx2_capp_bundle (
    bundle_id  VARCHAR(20) NOT NULL,
    param_code VARCHAR(20) NOT NULL,
    value      NUMERIC(20,6)
);

-- ----- CHIP bundle (levels 9-11)
INSERT INTO _txfx2_capp_bundle VALUES
    ('CHIP','WASTE_PCT',1.5),('CHIP','YIELD_PCT',98.5),
    ('CHIP','LABOR_HRS',0.0012),('CHIP','LABOR_RATE',22000),
    ('CHIP','LABOR_OVERHEAD_PCT',25),('CHIP','IND_LABOR_PCT',25),
    ('CHIP','MAT_OVERHEAD_PCT',3),('CHIP','MACHINE_PER_KG',1800),
    ('CHIP','DEPREC_PER_KG',450),('CHIP','ELEC_KWH',0.8),
    ('CHIP','ELEC_RATE',1500),('CHIP','STEAM_KG',2.5),
    ('CHIP','STEAM_RATE',800),('CHIP','WATER_M3',0.003),
    ('CHIP','WATER_RATE',12000),('CHIP','AIR_PER_KG',80),
    ('CHIP','MAINT_PER_KG',250),('CHIP','FACTORY_OH',600),
    ('CHIP','QC_PER_KG',30),('CHIP','PACK_PER_KG',0),
    ('CHIP','COST_RM_TOTAL',NULL),('CHIP','COST_RM_LOADED',NULL),
    ('CHIP','COST_ELEC',NULL),('CHIP','COST_LABOR',NULL),
    ('CHIP','COST_LABOR_FULL',NULL),('CHIP','COST_CONVERSION',NULL),
    ('CHIP','COST_STAGE_OUT',NULL),('CHIP','COST_STEAM',NULL),
    ('CHIP','COST_WATER',NULL),('CHIP','COST_UTIL',NULL),
    ('CHIP','COST_OVERHEAD',NULL),('CHIP','COST_AFTER_YLD',NULL);

-- ----- POY bundle (levels 4-8, spinning + drawing)
INSERT INTO _txfx2_capp_bundle VALUES
    ('POY','WASTE_PCT',2.0),('POY','YIELD_PCT',96.5),
    ('POY','LABOR_HRS',0.0035),('POY','LABOR_RATE',22000),
    ('POY','LABOR_OVERHEAD_PCT',30),('POY','IND_LABOR_PCT',30),
    ('POY','MAT_OVERHEAD_PCT',3),('POY','MACHINE_PER_KG',4200),
    ('POY','DEPREC_PER_KG',850),('POY','ELEC_KWH',1.8),
    ('POY','ELEC_RATE',1500),('POY','STEAM_KG',0.5),
    ('POY','STEAM_RATE',800),('POY','WATER_M3',0.002),
    ('POY','WATER_RATE',12000),('POY','AIR_PER_KG',220),
    ('POY','MAINT_PER_KG',380),('POY','FACTORY_OH',900),
    ('POY','QC_PER_KG',80),('POY','PACK_PER_KG',0),
    ('POY','COST_RM_TOTAL',NULL),('POY','COST_RM_LOADED',NULL),
    ('POY','COST_ELEC',NULL),('POY','COST_LABOR',NULL),
    ('POY','COST_LABOR_FULL',NULL),('POY','COST_CONVERSION',NULL),
    ('POY','COST_STAGE_OUT',NULL),('POY','COST_STEAM',NULL),
    ('POY','COST_WATER',NULL),('POY','COST_UTIL',NULL),
    ('POY','COST_OVERHEAD',NULL),('POY','COST_AFTER_YLD',NULL);

-- ----- DTY bundle (level 3)
INSERT INTO _txfx2_capp_bundle VALUES
    ('DTY','WASTE_PCT',1.8),('DTY','YIELD_PCT',97.5),
    ('DTY','LABOR_HRS',0.0045),('DTY','LABOR_RATE',22000),
    ('DTY','LABOR_OVERHEAD_PCT',30),('DTY','IND_LABOR_PCT',28),
    ('DTY','MAT_OVERHEAD_PCT',3),('DTY','MACHINE_PER_KG',5500),
    ('DTY','DEPREC_PER_KG',1100),('DTY','ELEC_KWH',1.2),
    ('DTY','ELEC_RATE',1500),('DTY','STEAM_KG',0),
    ('DTY','STEAM_RATE',800),('DTY','WATER_M3',0.001),
    ('DTY','WATER_RATE',12000),('DTY','AIR_PER_KG',300),
    ('DTY','MAINT_PER_KG',450),('DTY','FACTORY_OH',1100),
    ('DTY','QC_PER_KG',120),('DTY','PACK_PER_KG',250),
    ('DTY','COST_RM_TOTAL',NULL),('DTY','COST_RM_LOADED',NULL),
    ('DTY','COST_ELEC',NULL),('DTY','COST_LABOR',NULL),
    ('DTY','COST_LABOR_FULL',NULL),('DTY','COST_CONVERSION',NULL),
    ('DTY','COST_STAGE_OUT',NULL),('DTY','COST_STEAM',NULL),
    ('DTY','COST_WATER',NULL),('DTY','COST_UTIL',NULL),
    ('DTY','COST_OVERHEAD',NULL),('DTY','COST_AFTER_YLD',NULL);

-- ----- GREIGE bundle (level 2)
INSERT INTO _txfx2_capp_bundle VALUES
    ('GRG','WASTE_PCT',3.0),('GRG','YIELD_PCT',96.0),
    ('GRG','LABOR_HRS',0.0075),('GRG','LABOR_RATE',22000),
    ('GRG','LABOR_OVERHEAD_PCT',25),('GRG','IND_LABOR_PCT',25),
    ('GRG','MAT_OVERHEAD_PCT',2),('GRG','MACHINE_PER_KG',3800),
    ('GRG','DEPREC_PER_KG',700),('GRG','ELEC_KWH',0.6),
    ('GRG','ELEC_RATE',1500),('GRG','STEAM_KG',0),
    ('GRG','STEAM_RATE',800),('GRG','WATER_M3',0),
    ('GRG','WATER_RATE',12000),('GRG','AIR_PER_KG',100),
    ('GRG','MAINT_PER_KG',280),('GRG','FACTORY_OH',850),
    ('GRG','QC_PER_KG',90),('GRG','PACK_PER_KG',0),
    ('GRG','COST_RM_TOTAL',NULL),('GRG','COST_RM_LOADED',NULL),
    ('GRG','COST_ELEC',NULL),('GRG','COST_LABOR',NULL),
    ('GRG','COST_LABOR_FULL',NULL),('GRG','COST_CONVERSION',NULL),
    ('GRG','COST_STAGE_OUT',NULL),('GRG','COST_STEAM',NULL),
    ('GRG','COST_WATER',NULL),('GRG','COST_UTIL',NULL),
    ('GRG','COST_OVERHEAD',NULL),('GRG','COST_AFTER_YLD',NULL);

-- ----- DYED bundle (level 1)
INSERT INTO _txfx2_capp_bundle VALUES
    ('DYE','WASTE_PCT',4.0),('DYE','YIELD_PCT',94.5),
    ('DYE','LABOR_HRS',0.009),('DYE','LABOR_RATE',22000),
    ('DYE','LABOR_OVERHEAD_PCT',30),('DYE','IND_LABOR_PCT',30),
    ('DYE','MAT_OVERHEAD_PCT',4),('DYE','MACHINE_PER_KG',4500),
    ('DYE','DEPREC_PER_KG',900),('DYE','ELEC_KWH',0.9),
    ('DYE','ELEC_RATE',1500),('DYE','STEAM_KG',4.2),
    ('DYE','STEAM_RATE',800),('DYE','WATER_M3',0.12),
    ('DYE','WATER_RATE',12000),('DYE','AIR_PER_KG',150),
    ('DYE','MAINT_PER_KG',380),('DYE','FACTORY_OH',1500),
    ('DYE','QC_PER_KG',220),('DYE','PACK_PER_KG',0),
    ('DYE','COST_RM_TOTAL',NULL),('DYE','COST_RM_LOADED',NULL),
    ('DYE','COST_ELEC',NULL),('DYE','COST_LABOR',NULL),
    ('DYE','COST_LABOR_FULL',NULL),('DYE','COST_CONVERSION',NULL),
    ('DYE','COST_STAGE_OUT',NULL),('DYE','COST_STEAM',NULL),
    ('DYE','COST_WATER',NULL),('DYE','COST_UTIL',NULL),
    ('DYE','COST_OVERHEAD',NULL),('DYE','COST_AFTER_YLD',NULL);

-- ----- FG bundle (level 0)
INSERT INTO _txfx2_capp_bundle VALUES
    ('FG','WASTE_PCT',2.0),('FG','YIELD_PCT',98.0),
    ('FG','LABOR_HRS',0.005),('FG','LABOR_RATE',22000),
    ('FG','LABOR_OVERHEAD_PCT',30),('FG','IND_LABOR_PCT',30),
    ('FG','MAT_OVERHEAD_PCT',2),('FG','MACHINE_PER_KG',1200),
    ('FG','DEPREC_PER_KG',300),('FG','ELEC_KWH',0.3),
    ('FG','ELEC_RATE',1500),('FG','STEAM_KG',0),
    ('FG','STEAM_RATE',800),('FG','WATER_M3',0),
    ('FG','WATER_RATE',12000),('FG','AIR_PER_KG',50),
    ('FG','MAINT_PER_KG',150),('FG','FACTORY_OH',800),
    ('FG','QC_PER_KG',350),('FG','PACK_PER_KG',850),
    ('FG','MARGIN_PCT',18),
    ('FG','COST_RM_TOTAL',NULL),('FG','COST_RM_LOADED',NULL),
    ('FG','COST_ELEC',NULL),('FG','COST_LABOR',NULL),
    ('FG','COST_LABOR_FULL',NULL),('FG','COST_CONVERSION',NULL),
    ('FG','COST_STAGE_OUT',NULL),('FG','COST_STEAM',NULL),
    ('FG','COST_WATER',NULL),('FG','COST_UTIL',NULL),
    ('FG','COST_OVERHEAD',NULL),('FG','COST_AFTER_YLD',NULL),
    ('FG','SELLING_PRICE',NULL);

-- Map level -> bundle.
CREATE TEMP TABLE _txfx2_level_bundle (
    level INT PRIMARY KEY,
    bundle_id VARCHAR(20) NOT NULL
);
INSERT INTO _txfx2_level_bundle VALUES
    (11,'CHIP'),(10,'CHIP'),(9,'CHIP'),
    (8,'POY'),(7,'POY'),(6,'POY'),(5,'POY'),(4,'POY'),
    (3,'DTY'),
    (2,'GRG'),
    (1,'DYE'),
    (0,'FG');

-- Insert CAPP applicability rows.
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by
)
SELECT t.sys_id, p.id, FALSE, 'seed_000239'
  FROM _txfx2_products t
  JOIN _txfx2_level_bundle lb ON lb.level = t.level
  JOIN _txfx2_capp_bundle b   ON b.bundle_id = lb.bundle_id
  JOIN mst_parameter p        ON p.param_code = b.param_code AND p.deleted_at IS NULL
 WHERE t.sys_id IS NOT NULL
   AND NOT EXISTS (
       SELECT 1 FROM cost_product_applicable_param capp
        WHERE capp.capp_product_sys_id = t.sys_id
          AND capp.capp_param_id      = p.id
 );

-- Insert CPP value rows (skip NULL = calc-only).
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id,
    cpp_value_numeric,
    cpp_filled_by, cpp_created_by
)
SELECT t.sys_id, p.id, b.value, 'seed_000239', 'seed_000239'
  FROM _txfx2_products t
  JOIN _txfx2_level_bundle lb ON lb.level = t.level
  JOIN _txfx2_capp_bundle b   ON b.bundle_id = lb.bundle_id
  JOIN mst_parameter p        ON p.param_code = b.param_code AND p.deleted_at IS NULL
 WHERE t.sys_id IS NOT NULL
   AND b.value IS NOT NULL
   AND NOT EXISTS (
       SELECT 1 FROM cost_product_parameter cpp
        WHERE cpp.cpp_product_sys_id = t.sys_id
          AND cpp.cpp_param_id      = p.id
 );

COMMIT;
