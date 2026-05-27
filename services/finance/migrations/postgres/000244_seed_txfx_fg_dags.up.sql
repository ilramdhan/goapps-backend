-- 000244: Re-seed TXFX FG routes as self-contained multi-product DAGs.
--
-- Architecture (per user clarification):
--   * Each route_head = ONE FG. Its internal DAG spans the entire production
--     chain from raw chip (deepest level) up to FG (level 1).
--   * crs_route_level = 1 is the FG (top of canvas); higher level numbers are
--     upstream stages (closer to RMs).
--   * crs_route_seq numbers horizontal slots within a level (1..N side-by-side).
--   * crs_product_sys_id on each seq is the SPECIFIC intermediate produced at
--     that stage -- NEVER a copy of the head FG.
--   * PRODUCT-type RMs on a seq point to OTHER seqs in the SAME route as
--     upstream dependencies (matched by product_sys_id).
--   * ITEM-type RMs reference real cst_rm_cost.rm_code (sourced from
--     migration 000239 mapping):
--         DYE BLACK / RED-stand-in  -> 202503831 (PALIOGEN RED K)
--         DYE NAVY (BLUE)           -> 202007161 (BLUE MGTS-5054)
--         DYE RED                   -> 202007193 (RED MGTS 3059)
--         DYE YELLOW / OB           -> 202007205 (YELLOW MGTP-2159)
--         DYE GREY/BROWN            -> 202007166 (BROWN MGTS-6061)
--         DYE GENERAL AUX1          -> 202108613 (DYE0000045)
--         DYE GENERAL AUX2          -> 202411819 (DYE0000055)
--         MASTERBATCH / ADDITIVE    -> 202303677 (MBC0000291)
--
-- Topologies seeded (8 FG routes):
--   FG_JRS_BLK (16 seqs, 11 levels)  Greige 180gsm + black MB chip path
--   FG_JRS_NVY (16 seqs, 11 levels)  Greige 180gsm + navy MB chip path
--   FG_JRS_RED (16 seqs, 11 levels)  Greige 240gsm, DTY 150I + 300, natural FD chip
--   FG_PIQ_BLK (16 seqs, 11 levels)  Greige 200gsm pique, DTY 150N + 75I, black MB
--   FG_RIB_WHT (15 seqs, 11 levels)  Greige 220gsm rib, DTY 150I + 300, FD chip (OB dye)
--   FG_SPC_GRY (16 seqs, 11 levels)  Greige 280gsm, DTY 150N + 75BR (BR chip branch)
--   FG_TRC_RED (14 seqs, 11 levels)  Greige 120gsm tricot, DTY 75I only (linear at L3->L4)
--   FG_DTY075I_CN (12 seqs, 9 levels) Cone yarn FG (no greige/dye), DTY 75I top
--
-- Each route exhibits at least one split (e.g. preset -> HT + LT) and merges
-- (e.g. greige consumes two DTY variants; chip blend consumes raw + MB).
--
-- Idempotent: NOT EXISTS guard on cost_route_head.crh_product_sys_id, and
-- positions are deterministic.

BEGIN;

-- =============================================================================
-- 0. Per-run lookup of TXFX products -> sys_id.
-- =============================================================================
CREATE TEMP TABLE _txfx_dag_products (
    code   VARCHAR(30) PRIMARY KEY,
    sys_id BIGINT
);
INSERT INTO _txfx_dag_products(code, sys_id)
SELECT cpm_product_code, cpm_product_sys_id
  FROM cost_product_master
 WHERE cpm_product_code LIKE 'TXFX\_%' ESCAPE '\';

-- =============================================================================
-- 1. Seq spec table: (fg_code, level, seq, product_code, route_name).
--    crs_position_x / y are derived in the loop from level + seq slot count.
-- =============================================================================
CREATE TEMP TABLE _dag_seqs (
    fg_code         VARCHAR(30) NOT NULL,
    lvl             INT NOT NULL,
    seq             INT NOT NULL,
    product_code    VARCHAR(30) NOT NULL,
    route_name      VARCHAR(200) NOT NULL,
    PRIMARY KEY (fg_code, lvl, seq)
);

-- ---------- TXFX_FG_JRS_BLK ----------
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_JRS_BLK',  1, 1, 'TXFX_FG_JRS_BLK',    'FG Heat-Set + Pack Jersey Black'),
    ('TXFX_FG_JRS_BLK',  2, 1, 'TXFX_DY_JRS_BLK',    'Dye Jersey Black'),
    ('TXFX_FG_JRS_BLK',  3, 1, 'TXFX_G_JRS_180',     'Knit Greige Jersey 180gsm'),
    ('TXFX_FG_JRS_BLK',  4, 1, 'TXFX_DTY_150SD_N',   'DTY 150D SD Non-Intermingled'),
    ('TXFX_FG_JRS_BLK',  4, 2, 'TXFX_DTY_075SD_N',   'DTY 75D SD Non-Intermingled'),
    ('TXFX_FG_JRS_BLK',  5, 1, 'TXFX_DH_150SD',      'Drawn-HT 150D SD'),
    ('TXFX_FG_JRS_BLK',  5, 2, 'TXFX_DL_150SD',      'Drawn-LT 150D SD'),
    ('TXFX_FG_JRS_BLK',  5, 3, 'TXFX_DH_075SD',      'Drawn-HT 75D SD'),
    ('TXFX_FG_JRS_BLK',  5, 4, 'TXFX_DL_075SD',      'Drawn-LT 75D SD'),
    ('TXFX_FG_JRS_BLK',  6, 1, 'TXFX_DP_150SD',      'Drawn Pre-set 150D SD'),
    ('TXFX_FG_JRS_BLK',  6, 2, 'TXFX_DP_075SD',      'Drawn Pre-set 75D SD'),
    ('TXFX_FG_JRS_BLK',  7, 1, 'TXFX_P_150SD',       'POY 150D/48F Semi-Dull'),
    ('TXFX_FG_JRS_BLK',  7, 2, 'TXFX_P_075SD',       'POY 75D/36F Semi-Dull'),
    ('TXFX_FG_JRS_BLK',  8, 1, 'TXFX_PU_SD',         'Undrawn POY Semi-Dull'),
    ('TXFX_FG_JRS_BLK',  9, 1, 'TXFX_C3_SD',         'Chip SD dried+crystallized'),
    ('TXFX_FG_JRS_BLK', 10, 1, 'TXFX_C2_SD',         'Chip SD blend (raw + black MB)'),
    ('TXFX_FG_JRS_BLK', 11, 1, 'TXFX_C1_SD',         'Chip SD raw (polymerized)'),
    ('TXFX_FG_JRS_BLK', 11, 2, 'TXFX_C1_MB_BLK',     'Black Masterbatch chip');

-- ---------- TXFX_FG_JRS_NVY ----------  (same as BLK but navy MB + DY_JRS_NVY)
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_JRS_NVY',  1, 1, 'TXFX_FG_JRS_NVY',    'FG Heat-Set + Pack Jersey Navy'),
    ('TXFX_FG_JRS_NVY',  2, 1, 'TXFX_DY_JRS_NVY',    'Dye Jersey Navy'),
    ('TXFX_FG_JRS_NVY',  3, 1, 'TXFX_G_JRS_180',     'Knit Greige Jersey 180gsm'),
    ('TXFX_FG_JRS_NVY',  4, 1, 'TXFX_DTY_150SD_N',   'DTY 150D SD Non-Intermingled'),
    ('TXFX_FG_JRS_NVY',  4, 2, 'TXFX_DTY_075SD_N',   'DTY 75D SD Non-Intermingled'),
    ('TXFX_FG_JRS_NVY',  5, 1, 'TXFX_DH_150SD',      'Drawn-HT 150D SD'),
    ('TXFX_FG_JRS_NVY',  5, 2, 'TXFX_DL_150SD',      'Drawn-LT 150D SD'),
    ('TXFX_FG_JRS_NVY',  5, 3, 'TXFX_DH_075SD',      'Drawn-HT 75D SD'),
    ('TXFX_FG_JRS_NVY',  5, 4, 'TXFX_DL_075SD',      'Drawn-LT 75D SD'),
    ('TXFX_FG_JRS_NVY',  6, 1, 'TXFX_DP_150SD',      'Drawn Pre-set 150D SD'),
    ('TXFX_FG_JRS_NVY',  6, 2, 'TXFX_DP_075SD',      'Drawn Pre-set 75D SD'),
    ('TXFX_FG_JRS_NVY',  7, 1, 'TXFX_P_150SD',       'POY 150D/48F Semi-Dull'),
    ('TXFX_FG_JRS_NVY',  7, 2, 'TXFX_P_075SD',       'POY 75D/36F Semi-Dull'),
    ('TXFX_FG_JRS_NVY',  8, 1, 'TXFX_PU_SD',         'Undrawn POY Semi-Dull'),
    ('TXFX_FG_JRS_NVY',  9, 1, 'TXFX_C3_SD',         'Chip SD dried+crystallized'),
    ('TXFX_FG_JRS_NVY', 10, 1, 'TXFX_C2_SD',         'Chip SD blend (raw + navy MB)'),
    ('TXFX_FG_JRS_NVY', 11, 1, 'TXFX_C1_SD',         'Chip SD raw (polymerized)'),
    ('TXFX_FG_JRS_NVY', 11, 2, 'TXFX_C1_MB_NVY',     'Navy Masterbatch chip');

-- ---------- TXFX_FG_JRS_RED ----------  Greige 240, DTY 150I + 300SD, FD chip
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_JRS_RED',  1, 1, 'TXFX_FG_JRS_RED',    'FG Heat-Set + Pack Jersey Red'),
    ('TXFX_FG_JRS_RED',  2, 1, 'TXFX_DY_JRS_RED',    'Dye Jersey Red'),
    ('TXFX_FG_JRS_RED',  3, 1, 'TXFX_G_JRS_240',     'Knit Greige Jersey 240gsm'),
    ('TXFX_FG_JRS_RED',  4, 1, 'TXFX_DTY_150SD_I',   'DTY 150D SD Intermingled'),
    ('TXFX_FG_JRS_RED',  4, 2, 'TXFX_DTY_300SD',     'DTY 300D SD'),
    ('TXFX_FG_JRS_RED',  5, 1, 'TXFX_DH_150SD',      'Drawn-HT 150D SD'),
    ('TXFX_FG_JRS_RED',  5, 2, 'TXFX_DL_150SD',      'Drawn-LT 150D SD'),
    ('TXFX_FG_JRS_RED',  5, 3, 'TXFX_DH_300SD',      'Drawn-HT 300D SD'),
    ('TXFX_FG_JRS_RED',  5, 4, 'TXFX_DL_300SD',      'Drawn-LT 300D SD'),
    ('TXFX_FG_JRS_RED',  6, 1, 'TXFX_DP_150SD',      'Drawn Pre-set 150D SD'),
    ('TXFX_FG_JRS_RED',  6, 2, 'TXFX_DP_300SD',      'Drawn Pre-set 300D SD'),
    ('TXFX_FG_JRS_RED',  7, 1, 'TXFX_P_150SD',       'POY 150D/48F Semi-Dull'),
    ('TXFX_FG_JRS_RED',  7, 2, 'TXFX_P_300SD',       'POY 300D/96F Semi-Dull'),
    ('TXFX_FG_JRS_RED',  8, 1, 'TXFX_PU_FD',         'Undrawn POY Full-Dull'),
    ('TXFX_FG_JRS_RED',  9, 1, 'TXFX_C3_FD',         'Chip FD dried+crystallized'),
    ('TXFX_FG_JRS_RED', 10, 1, 'TXFX_C2_FD',         'Chip FD blend (natural, no MB)'),
    ('TXFX_FG_JRS_RED', 11, 1, 'TXFX_C1_FD',         'Chip FD raw (polymerized)');

-- ---------- TXFX_FG_PIQ_BLK ----------  Greige 200, DTY 150N + 75I, black MB
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_PIQ_BLK',  1, 1, 'TXFX_FG_PIQ_BLK',    'FG Heat-Set + Pack Pique Black'),
    ('TXFX_FG_PIQ_BLK',  2, 1, 'TXFX_DY_PIQ_BLK',    'Dye Pique Black'),
    ('TXFX_FG_PIQ_BLK',  3, 1, 'TXFX_G_PIQ_200',     'Knit Greige Pique 200gsm'),
    ('TXFX_FG_PIQ_BLK',  4, 1, 'TXFX_DTY_150SD_N',   'DTY 150D SD Non-Intermingled'),
    ('TXFX_FG_PIQ_BLK',  4, 2, 'TXFX_DTY_075SD_I',   'DTY 75D SD Intermingled'),
    ('TXFX_FG_PIQ_BLK',  5, 1, 'TXFX_DH_150SD',      'Drawn-HT 150D SD'),
    ('TXFX_FG_PIQ_BLK',  5, 2, 'TXFX_DL_150SD',      'Drawn-LT 150D SD'),
    ('TXFX_FG_PIQ_BLK',  5, 3, 'TXFX_DH_075SD',      'Drawn-HT 75D SD'),
    ('TXFX_FG_PIQ_BLK',  5, 4, 'TXFX_DL_075SD',      'Drawn-LT 75D SD'),
    ('TXFX_FG_PIQ_BLK',  6, 1, 'TXFX_DP_150SD',      'Drawn Pre-set 150D SD'),
    ('TXFX_FG_PIQ_BLK',  6, 2, 'TXFX_DP_075SD',      'Drawn Pre-set 75D SD'),
    ('TXFX_FG_PIQ_BLK',  7, 1, 'TXFX_P_150SD',       'POY 150D/48F Semi-Dull'),
    ('TXFX_FG_PIQ_BLK',  7, 2, 'TXFX_P_075SD',       'POY 75D/36F Semi-Dull'),
    ('TXFX_FG_PIQ_BLK',  8, 1, 'TXFX_PU_SD',         'Undrawn POY Semi-Dull'),
    ('TXFX_FG_PIQ_BLK',  9, 1, 'TXFX_C3_SD',         'Chip SD dried+crystallized'),
    ('TXFX_FG_PIQ_BLK', 10, 1, 'TXFX_C2_SD',         'Chip SD blend (raw + black MB)'),
    ('TXFX_FG_PIQ_BLK', 11, 1, 'TXFX_C1_SD',         'Chip SD raw (polymerized)'),
    ('TXFX_FG_PIQ_BLK', 11, 2, 'TXFX_C1_MB_BLK',     'Black Masterbatch chip');

-- ---------- TXFX_FG_RIB_WHT ----------  Greige 220, DTY 150I + 300, FD chip (no MB)
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_RIB_WHT',  1, 1, 'TXFX_FG_RIB_WHT',    'FG Heat-Set + Pack Rib Optical White'),
    ('TXFX_FG_RIB_WHT',  2, 1, 'TXFX_DY_RIB_WHT',    'Bleach + Optical-Brighten Rib'),
    ('TXFX_FG_RIB_WHT',  3, 1, 'TXFX_G_RIB_220',     'Knit Greige Rib 220gsm'),
    ('TXFX_FG_RIB_WHT',  4, 1, 'TXFX_DTY_150SD_I',   'DTY 150D SD Intermingled'),
    ('TXFX_FG_RIB_WHT',  4, 2, 'TXFX_DTY_300SD',     'DTY 300D SD'),
    ('TXFX_FG_RIB_WHT',  5, 1, 'TXFX_DH_150SD',      'Drawn-HT 150D SD'),
    ('TXFX_FG_RIB_WHT',  5, 2, 'TXFX_DL_150SD',      'Drawn-LT 150D SD'),
    ('TXFX_FG_RIB_WHT',  5, 3, 'TXFX_DH_300SD',      'Drawn-HT 300D SD'),
    ('TXFX_FG_RIB_WHT',  5, 4, 'TXFX_DL_300SD',      'Drawn-LT 300D SD'),
    ('TXFX_FG_RIB_WHT',  6, 1, 'TXFX_DP_150SD',      'Drawn Pre-set 150D SD'),
    ('TXFX_FG_RIB_WHT',  6, 2, 'TXFX_DP_300SD',      'Drawn Pre-set 300D SD'),
    ('TXFX_FG_RIB_WHT',  7, 1, 'TXFX_P_150SD',       'POY 150D/48F Semi-Dull'),
    ('TXFX_FG_RIB_WHT',  7, 2, 'TXFX_P_300SD',       'POY 300D/96F Semi-Dull'),
    ('TXFX_FG_RIB_WHT',  8, 1, 'TXFX_PU_FD',         'Undrawn POY Full-Dull'),
    ('TXFX_FG_RIB_WHT',  9, 1, 'TXFX_C3_FD',         'Chip FD dried+crystallized'),
    ('TXFX_FG_RIB_WHT', 10, 1, 'TXFX_C2_FD',         'Chip FD blend (natural)'),
    ('TXFX_FG_RIB_WHT', 11, 1, 'TXFX_C1_FD',         'Chip FD raw (polymerized)');

-- ---------- TXFX_FG_SPC_GRY ----------  Greige 280, DTY 150N + 75BR, BR chip branch
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_SPC_GRY',  1, 1, 'TXFX_FG_SPC_GRY',    'FG Heat-Set + Pack Spacer Grey'),
    ('TXFX_FG_SPC_GRY',  2, 1, 'TXFX_DY_SPC_GRY',    'Dye Spacer Grey'),
    ('TXFX_FG_SPC_GRY',  3, 1, 'TXFX_G_SPC_280',     'Knit Greige Spacer 280gsm'),
    ('TXFX_FG_SPC_GRY',  4, 1, 'TXFX_DTY_150SD_N',   'DTY 150D SD Non-Intermingled'),
    ('TXFX_FG_SPC_GRY',  4, 2, 'TXFX_DTY_075BR',     'DTY 75D Bright'),
    ('TXFX_FG_SPC_GRY',  5, 1, 'TXFX_DH_150SD',      'Drawn-HT 150D SD'),
    ('TXFX_FG_SPC_GRY',  5, 2, 'TXFX_DL_150SD',      'Drawn-LT 150D SD'),
    ('TXFX_FG_SPC_GRY',  6, 1, 'TXFX_DP_150SD',      'Drawn Pre-set 150D SD'),
    ('TXFX_FG_SPC_GRY',  6, 2, 'TXFX_DP_075BR',      'Drawn Pre-set 75D BR'),
    ('TXFX_FG_SPC_GRY',  7, 1, 'TXFX_P_150SD',       'POY 150D/48F Semi-Dull'),
    ('TXFX_FG_SPC_GRY',  7, 2, 'TXFX_P_075BR',       'POY 75D/36F Bright'),
    ('TXFX_FG_SPC_GRY',  8, 1, 'TXFX_PU_SD',         'Undrawn POY Semi-Dull'),
    ('TXFX_FG_SPC_GRY',  8, 2, 'TXFX_PU_BR',         'Undrawn POY Bright'),
    ('TXFX_FG_SPC_GRY',  9, 1, 'TXFX_C3_SD',         'Chip SD dried+crystallized'),
    ('TXFX_FG_SPC_GRY',  9, 2, 'TXFX_C3_BR',         'Chip BR dried+crystallized'),
    ('TXFX_FG_SPC_GRY', 10, 1, 'TXFX_C2_SD',         'Chip SD blend'),
    ('TXFX_FG_SPC_GRY', 10, 2, 'TXFX_C2_BR',         'Chip BR blend'),
    ('TXFX_FG_SPC_GRY', 11, 1, 'TXFX_C1_SD',         'Chip SD raw'),
    ('TXFX_FG_SPC_GRY', 11, 2, 'TXFX_C1_BR',         'Chip BR raw');

-- ---------- TXFX_FG_TRC_RED ----------  Greige 120, DTY 75I only (linear at L3)
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_TRC_RED',  1, 1, 'TXFX_FG_TRC_RED',    'FG Heat-Set + Pack Tricot Red'),
    ('TXFX_FG_TRC_RED',  2, 1, 'TXFX_DY_TRC_RED',    'Dye Tricot Red'),
    ('TXFX_FG_TRC_RED',  3, 1, 'TXFX_G_TRC_120',     'Knit Greige Tricot 120gsm'),
    ('TXFX_FG_TRC_RED',  4, 1, 'TXFX_DTY_075SD_I',   'DTY 75D SD Intermingled'),
    ('TXFX_FG_TRC_RED',  5, 1, 'TXFX_DH_075SD',      'Drawn-HT 75D SD'),
    ('TXFX_FG_TRC_RED',  5, 2, 'TXFX_DL_075SD',      'Drawn-LT 75D SD'),
    ('TXFX_FG_TRC_RED',  6, 1, 'TXFX_DP_075SD',      'Drawn Pre-set 75D SD'),
    ('TXFX_FG_TRC_RED',  7, 1, 'TXFX_P_075SD',       'POY 75D/36F Semi-Dull'),
    ('TXFX_FG_TRC_RED',  8, 1, 'TXFX_PU_SD',         'Undrawn POY Semi-Dull'),
    ('TXFX_FG_TRC_RED',  9, 1, 'TXFX_C3_SD',         'Chip SD dried+crystallized'),
    ('TXFX_FG_TRC_RED', 10, 1, 'TXFX_C2_SD',         'Chip SD blend (raw + black MB)'),
    ('TXFX_FG_TRC_RED', 11, 1, 'TXFX_C1_SD',         'Chip SD raw'),
    ('TXFX_FG_TRC_RED', 11, 2, 'TXFX_C1_MB_BLK',     'Black Masterbatch chip (red dyed-on)');

-- ---------- TXFX_FG_DTY075I_CN ----------  Cone yarn: shallower DAG (no greige/dyed)
INSERT INTO _dag_seqs VALUES
    ('TXFX_FG_DTY075I_CN', 1, 1, 'TXFX_FG_DTY075I_CN', 'FG Cone-wind + Pack DTY 75I'),
    ('TXFX_FG_DTY075I_CN', 2, 1, 'TXFX_DTY_075SD_I',   'DTY 75D SD Intermingled'),
    ('TXFX_FG_DTY075I_CN', 3, 1, 'TXFX_DH_075SD',      'Drawn-HT 75D SD'),
    ('TXFX_FG_DTY075I_CN', 3, 2, 'TXFX_DL_075SD',      'Drawn-LT 75D SD'),
    ('TXFX_FG_DTY075I_CN', 4, 1, 'TXFX_DP_075SD',      'Drawn Pre-set 75D SD'),
    ('TXFX_FG_DTY075I_CN', 5, 1, 'TXFX_P_075SD',       'POY 75D/36F Semi-Dull'),
    ('TXFX_FG_DTY075I_CN', 6, 1, 'TXFX_PU_SD',         'Undrawn POY Semi-Dull'),
    ('TXFX_FG_DTY075I_CN', 7, 1, 'TXFX_C3_SD',         'Chip SD dried+crystallized'),
    ('TXFX_FG_DTY075I_CN', 8, 1, 'TXFX_C2_SD',         'Chip SD blend'),
    ('TXFX_FG_DTY075I_CN', 9, 1, 'TXFX_C1_SD',         'Chip SD raw'),
    ('TXFX_FG_DTY075I_CN', 9, 2, 'TXFX_C1_MB_BLK',     'Black Masterbatch chip');

-- =============================================================================
-- 2. RM spec table: (fg_code, target_level, target_seq, rm_type, rm_ref, ratio, rm_name).
--    For PRODUCT rms, rm_ref is a TXFX_* product_code (resolved against the
--    same route's seq table). For ITEM rms, rm_ref is an rm_code (string).
-- =============================================================================
CREATE TEMP TABLE _dag_rms (
    fg_code        VARCHAR(30) NOT NULL,
    t_lvl          INT NOT NULL,
    t_seq          INT NOT NULL,
    rm_type        VARCHAR(10) NOT NULL,
    rm_ref         VARCHAR(40) NOT NULL,
    ratio          NUMERIC(10,6) NOT NULL,
    rm_name        VARCHAR(200) NOT NULL
);

-- =============================================================================
-- ITEM-RM helper macro using INSERTs. Each FG mostly shares the "chip raw"
-- bottom (which gets generic chemicals stand-in via masterbatch ratio plus
-- one auxiliary ITEM on the chip-blend stage). At the dye stage each FG gets
-- a colour-specific dyestuff + a generic aux.
-- =============================================================================

-- ============== TXFX_FG_JRS_BLK ==============
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_JRS_BLK',  1, 1, 'PRODUCT', 'TXFX_DY_JRS_BLK',   1.020, 'Dyed jersey input'),
    -- Dye seq
    ('TXFX_FG_JRS_BLK',  2, 1, 'PRODUCT', 'TXFX_G_JRS_180',    1.050, 'Greige input to dye'),
    ('TXFX_FG_JRS_BLK',  2, 1, 'ITEM',    '202503831',         0.040, 'Black dyestuff'),
    ('TXFX_FG_JRS_BLK',  2, 1, 'ITEM',    '202108613',         0.010, 'Dye auxiliary'),
    -- Greige seq (MERGE)
    ('TXFX_FG_JRS_BLK',  3, 1, 'PRODUCT', 'TXFX_DTY_150SD_N',  0.650, 'DTY 150 yarn input'),
    ('TXFX_FG_JRS_BLK',  3, 1, 'PRODUCT', 'TXFX_DTY_075SD_N',  0.370, 'DTY 75 yarn input'),
    -- DTY 150 (MERGE of HT+LT)
    ('TXFX_FG_JRS_BLK',  4, 1, 'PRODUCT', 'TXFX_DH_150SD',     0.500, 'HT 150 yarn'),
    ('TXFX_FG_JRS_BLK',  4, 1, 'PRODUCT', 'TXFX_DL_150SD',     0.500, 'LT 150 yarn'),
    -- DTY 75 (MERGE of HT+LT)
    ('TXFX_FG_JRS_BLK',  4, 2, 'PRODUCT', 'TXFX_DH_075SD',     0.500, 'HT 75 yarn'),
    ('TXFX_FG_JRS_BLK',  4, 2, 'PRODUCT', 'TXFX_DL_075SD',     0.500, 'LT 75 yarn'),
    -- Drawn-HT 150 -> Preset 150
    ('TXFX_FG_JRS_BLK',  5, 1, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_JRS_BLK',  5, 2, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_JRS_BLK',  5, 3, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_JRS_BLK',  5, 4, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    -- Preset -> POY
    ('TXFX_FG_JRS_BLK',  6, 1, 'PRODUCT', 'TXFX_P_150SD',      1.020, 'POY 150 input'),
    ('TXFX_FG_JRS_BLK',  6, 2, 'PRODUCT', 'TXFX_P_075SD',      1.020, 'POY 75 input'),
    -- POY -> Undrawn (MERGE)
    ('TXFX_FG_JRS_BLK',  7, 1, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_JRS_BLK',  7, 2, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    -- Undrawn -> C3 dried
    ('TXFX_FG_JRS_BLK',  8, 1, 'PRODUCT', 'TXFX_C3_SD',        1.000, 'Dried chip input'),
    -- C3 -> C2 blend
    ('TXFX_FG_JRS_BLK',  9, 1, 'PRODUCT', 'TXFX_C2_SD',        1.000, 'Blended chip input'),
    -- C2 blend (MERGE: raw + MB)
    ('TXFX_FG_JRS_BLK', 10, 1, 'PRODUCT', 'TXFX_C1_SD',        0.990, 'Raw chip input'),
    ('TXFX_FG_JRS_BLK', 10, 1, 'PRODUCT', 'TXFX_C1_MB_BLK',    0.020, 'Black masterbatch'),
    ('TXFX_FG_JRS_BLK', 10, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    -- C1 raw chemicals (ITEM only)
    ('TXFX_FG_JRS_BLK', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1 (MBC stand-in)'),
    ('TXFX_FG_JRS_BLK', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_JRS_BLK', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in'),
    -- MB black chemicals
    ('TXFX_FG_JRS_BLK', 11, 2, 'ITEM',    '202503831',         0.500, 'Black pigment'),
    ('TXFX_FG_JRS_BLK', 11, 2, 'ITEM',    '202303677',         0.500, 'MB carrier resin');

-- ============== TXFX_FG_JRS_NVY ==============
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_JRS_NVY',  1, 1, 'PRODUCT', 'TXFX_DY_JRS_NVY',   1.020, 'Dyed jersey input'),
    ('TXFX_FG_JRS_NVY',  2, 1, 'PRODUCT', 'TXFX_G_JRS_180',    1.050, 'Greige input to dye'),
    ('TXFX_FG_JRS_NVY',  2, 1, 'ITEM',    '202007161',         0.035, 'Navy/blue dyestuff'),
    ('TXFX_FG_JRS_NVY',  2, 1, 'ITEM',    '202411819',         0.010, 'Dye auxiliary'),
    ('TXFX_FG_JRS_NVY',  3, 1, 'PRODUCT', 'TXFX_DTY_150SD_N',  0.650, 'DTY 150 yarn input'),
    ('TXFX_FG_JRS_NVY',  3, 1, 'PRODUCT', 'TXFX_DTY_075SD_N',  0.370, 'DTY 75 yarn input'),
    ('TXFX_FG_JRS_NVY',  4, 1, 'PRODUCT', 'TXFX_DH_150SD',     0.500, 'HT 150 yarn'),
    ('TXFX_FG_JRS_NVY',  4, 1, 'PRODUCT', 'TXFX_DL_150SD',     0.500, 'LT 150 yarn'),
    ('TXFX_FG_JRS_NVY',  4, 2, 'PRODUCT', 'TXFX_DH_075SD',     0.500, 'HT 75 yarn'),
    ('TXFX_FG_JRS_NVY',  4, 2, 'PRODUCT', 'TXFX_DL_075SD',     0.500, 'LT 75 yarn'),
    ('TXFX_FG_JRS_NVY',  5, 1, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_JRS_NVY',  5, 2, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_JRS_NVY',  5, 3, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_JRS_NVY',  5, 4, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_JRS_NVY',  6, 1, 'PRODUCT', 'TXFX_P_150SD',      1.020, 'POY 150 input'),
    ('TXFX_FG_JRS_NVY',  6, 2, 'PRODUCT', 'TXFX_P_075SD',      1.020, 'POY 75 input'),
    ('TXFX_FG_JRS_NVY',  7, 1, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_JRS_NVY',  7, 2, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_JRS_NVY',  8, 1, 'PRODUCT', 'TXFX_C3_SD',        1.000, 'Dried chip input'),
    ('TXFX_FG_JRS_NVY',  9, 1, 'PRODUCT', 'TXFX_C2_SD',        1.000, 'Blended chip input'),
    ('TXFX_FG_JRS_NVY', 10, 1, 'PRODUCT', 'TXFX_C1_SD',        0.990, 'Raw chip input'),
    ('TXFX_FG_JRS_NVY', 10, 1, 'PRODUCT', 'TXFX_C1_MB_NVY',    0.020, 'Navy masterbatch'),
    ('TXFX_FG_JRS_NVY', 10, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    ('TXFX_FG_JRS_NVY', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1'),
    ('TXFX_FG_JRS_NVY', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_JRS_NVY', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in'),
    ('TXFX_FG_JRS_NVY', 11, 2, 'ITEM',    '202007161',         0.500, 'Navy pigment'),
    ('TXFX_FG_JRS_NVY', 11, 2, 'ITEM',    '202303677',         0.500, 'MB carrier resin');

-- ============== TXFX_FG_JRS_RED ==============
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_JRS_RED',  1, 1, 'PRODUCT', 'TXFX_DY_JRS_RED',   1.020, 'Dyed jersey input'),
    ('TXFX_FG_JRS_RED',  2, 1, 'PRODUCT', 'TXFX_G_JRS_240',    1.050, 'Greige 240 input'),
    ('TXFX_FG_JRS_RED',  2, 1, 'ITEM',    '202007193',         0.045, 'Red dyestuff'),
    ('TXFX_FG_JRS_RED',  2, 1, 'ITEM',    '202108613',         0.010, 'Dye auxiliary'),
    ('TXFX_FG_JRS_RED',  3, 1, 'PRODUCT', 'TXFX_DTY_150SD_I',  0.620, 'DTY 150 IM input'),
    ('TXFX_FG_JRS_RED',  3, 1, 'PRODUCT', 'TXFX_DTY_300SD',    0.410, 'DTY 300 input'),
    ('TXFX_FG_JRS_RED',  4, 1, 'PRODUCT', 'TXFX_DH_150SD',     0.520, 'HT 150 yarn'),
    ('TXFX_FG_JRS_RED',  4, 1, 'PRODUCT', 'TXFX_DL_150SD',     0.510, 'LT 150 yarn'),
    ('TXFX_FG_JRS_RED',  4, 2, 'PRODUCT', 'TXFX_DH_300SD',     0.560, 'HT 300 yarn'),
    ('TXFX_FG_JRS_RED',  4, 2, 'PRODUCT', 'TXFX_DL_300SD',     0.470, 'LT 300 yarn'),
    ('TXFX_FG_JRS_RED',  5, 1, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_JRS_RED',  5, 2, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_JRS_RED',  5, 3, 'PRODUCT', 'TXFX_DP_300SD',     1.000, 'Preset 300 input'),
    ('TXFX_FG_JRS_RED',  5, 4, 'PRODUCT', 'TXFX_DP_300SD',     1.000, 'Preset 300 input'),
    ('TXFX_FG_JRS_RED',  6, 1, 'PRODUCT', 'TXFX_P_150SD',      1.020, 'POY 150 input'),
    ('TXFX_FG_JRS_RED',  6, 2, 'PRODUCT', 'TXFX_P_300SD',      1.020, 'POY 300 input'),
    ('TXFX_FG_JRS_RED',  7, 1, 'PRODUCT', 'TXFX_PU_FD',        1.000, 'Undrawn FD input'),
    ('TXFX_FG_JRS_RED',  7, 2, 'PRODUCT', 'TXFX_PU_FD',        1.000, 'Undrawn FD input'),
    ('TXFX_FG_JRS_RED',  8, 1, 'PRODUCT', 'TXFX_C3_FD',        1.000, 'Dried FD chip input'),
    ('TXFX_FG_JRS_RED',  9, 1, 'PRODUCT', 'TXFX_C2_FD',        1.000, 'Blended FD chip input'),
    ('TXFX_FG_JRS_RED', 10, 1, 'PRODUCT', 'TXFX_C1_FD',        1.000, 'Raw FD chip input'),
    ('TXFX_FG_JRS_RED', 10, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    ('TXFX_FG_JRS_RED', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1'),
    ('TXFX_FG_JRS_RED', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_JRS_RED', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in');

-- ============== TXFX_FG_PIQ_BLK ==============
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_PIQ_BLK',  1, 1, 'PRODUCT', 'TXFX_DY_PIQ_BLK',   1.020, 'Dyed pique input'),
    ('TXFX_FG_PIQ_BLK',  2, 1, 'PRODUCT', 'TXFX_G_PIQ_200',    1.050, 'Greige pique input'),
    ('TXFX_FG_PIQ_BLK',  2, 1, 'ITEM',    '202503831',         0.040, 'Black dyestuff'),
    ('TXFX_FG_PIQ_BLK',  2, 1, 'ITEM',    '202108613',         0.010, 'Dye auxiliary'),
    ('TXFX_FG_PIQ_BLK',  3, 1, 'PRODUCT', 'TXFX_DTY_150SD_N',  0.580, 'DTY 150 NIM input'),
    ('TXFX_FG_PIQ_BLK',  3, 1, 'PRODUCT', 'TXFX_DTY_075SD_I',  0.430, 'DTY 75 IM input'),
    ('TXFX_FG_PIQ_BLK',  4, 1, 'PRODUCT', 'TXFX_DH_150SD',     0.500, 'HT 150 yarn'),
    ('TXFX_FG_PIQ_BLK',  4, 1, 'PRODUCT', 'TXFX_DL_150SD',     0.500, 'LT 150 yarn'),
    ('TXFX_FG_PIQ_BLK',  4, 2, 'PRODUCT', 'TXFX_DH_075SD',     0.510, 'HT 75 yarn'),
    ('TXFX_FG_PIQ_BLK',  4, 2, 'PRODUCT', 'TXFX_DL_075SD',     0.510, 'LT 75 yarn'),
    ('TXFX_FG_PIQ_BLK',  5, 1, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_PIQ_BLK',  5, 2, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_PIQ_BLK',  5, 3, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_PIQ_BLK',  5, 4, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_PIQ_BLK',  6, 1, 'PRODUCT', 'TXFX_P_150SD',      1.020, 'POY 150 input'),
    ('TXFX_FG_PIQ_BLK',  6, 2, 'PRODUCT', 'TXFX_P_075SD',      1.020, 'POY 75 input'),
    ('TXFX_FG_PIQ_BLK',  7, 1, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_PIQ_BLK',  7, 2, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_PIQ_BLK',  8, 1, 'PRODUCT', 'TXFX_C3_SD',        1.000, 'Dried chip input'),
    ('TXFX_FG_PIQ_BLK',  9, 1, 'PRODUCT', 'TXFX_C2_SD',        1.000, 'Blended chip input'),
    ('TXFX_FG_PIQ_BLK', 10, 1, 'PRODUCT', 'TXFX_C1_SD',        0.990, 'Raw chip input'),
    ('TXFX_FG_PIQ_BLK', 10, 1, 'PRODUCT', 'TXFX_C1_MB_BLK',    0.020, 'Black masterbatch'),
    ('TXFX_FG_PIQ_BLK', 10, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    ('TXFX_FG_PIQ_BLK', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1'),
    ('TXFX_FG_PIQ_BLK', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_PIQ_BLK', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in'),
    ('TXFX_FG_PIQ_BLK', 11, 2, 'ITEM',    '202503831',         0.500, 'Black pigment'),
    ('TXFX_FG_PIQ_BLK', 11, 2, 'ITEM',    '202303677',         0.500, 'MB carrier resin');

-- ============== TXFX_FG_RIB_WHT ==============  (no MB; FD chip path)
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_RIB_WHT',  1, 1, 'PRODUCT', 'TXFX_DY_RIB_WHT',   1.020, 'Bleached rib input'),
    ('TXFX_FG_RIB_WHT',  2, 1, 'PRODUCT', 'TXFX_G_RIB_220',    1.045, 'Greige rib input'),
    ('TXFX_FG_RIB_WHT',  2, 1, 'ITEM',    '202007205',         0.015, 'Optical brightener'),
    ('TXFX_FG_RIB_WHT',  2, 1, 'ITEM',    '202108613',         0.008, 'Bleach auxiliary'),
    ('TXFX_FG_RIB_WHT',  3, 1, 'PRODUCT', 'TXFX_DTY_150SD_I',  0.700, 'DTY 150 IM input'),
    ('TXFX_FG_RIB_WHT',  3, 1, 'PRODUCT', 'TXFX_DTY_300SD',    0.320, 'DTY 300 input'),
    ('TXFX_FG_RIB_WHT',  4, 1, 'PRODUCT', 'TXFX_DH_150SD',     0.520, 'HT 150 yarn'),
    ('TXFX_FG_RIB_WHT',  4, 1, 'PRODUCT', 'TXFX_DL_150SD',     0.510, 'LT 150 yarn'),
    ('TXFX_FG_RIB_WHT',  4, 2, 'PRODUCT', 'TXFX_DH_300SD',     0.560, 'HT 300 yarn'),
    ('TXFX_FG_RIB_WHT',  4, 2, 'PRODUCT', 'TXFX_DL_300SD',     0.470, 'LT 300 yarn'),
    ('TXFX_FG_RIB_WHT',  5, 1, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_RIB_WHT',  5, 2, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_RIB_WHT',  5, 3, 'PRODUCT', 'TXFX_DP_300SD',     1.000, 'Preset 300 input'),
    ('TXFX_FG_RIB_WHT',  5, 4, 'PRODUCT', 'TXFX_DP_300SD',     1.000, 'Preset 300 input'),
    ('TXFX_FG_RIB_WHT',  6, 1, 'PRODUCT', 'TXFX_P_150SD',      1.020, 'POY 150 input'),
    ('TXFX_FG_RIB_WHT',  6, 2, 'PRODUCT', 'TXFX_P_300SD',      1.020, 'POY 300 input'),
    ('TXFX_FG_RIB_WHT',  7, 1, 'PRODUCT', 'TXFX_PU_FD',        1.000, 'Undrawn FD input'),
    ('TXFX_FG_RIB_WHT',  7, 2, 'PRODUCT', 'TXFX_PU_FD',        1.000, 'Undrawn FD input'),
    ('TXFX_FG_RIB_WHT',  8, 1, 'PRODUCT', 'TXFX_C3_FD',        1.000, 'Dried FD chip input'),
    ('TXFX_FG_RIB_WHT',  9, 1, 'PRODUCT', 'TXFX_C2_FD',        1.000, 'Blended FD chip input'),
    ('TXFX_FG_RIB_WHT', 10, 1, 'PRODUCT', 'TXFX_C1_FD',        1.000, 'Raw FD chip input'),
    ('TXFX_FG_RIB_WHT', 10, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    ('TXFX_FG_RIB_WHT', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1'),
    ('TXFX_FG_RIB_WHT', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_RIB_WHT', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in');

-- ============== TXFX_FG_SPC_GRY ==============  Greige 280, SD + BR chip branches
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_SPC_GRY',  1, 1, 'PRODUCT', 'TXFX_DY_SPC_GRY',   1.020, 'Dyed spacer input'),
    ('TXFX_FG_SPC_GRY',  2, 1, 'PRODUCT', 'TXFX_G_SPC_280',    1.045, 'Greige spacer input'),
    ('TXFX_FG_SPC_GRY',  2, 1, 'ITEM',    '202007166',         0.025, 'Grey dyestuff'),
    ('TXFX_FG_SPC_GRY',  2, 1, 'ITEM',    '202411819',         0.010, 'Dye auxiliary'),
    ('TXFX_FG_SPC_GRY',  3, 1, 'PRODUCT', 'TXFX_DTY_150SD_N',  0.620, 'DTY 150 NIM input'),
    ('TXFX_FG_SPC_GRY',  3, 1, 'PRODUCT', 'TXFX_DTY_075BR',    0.400, 'DTY 75 Bright input'),
    -- DTY 150 SD branch
    ('TXFX_FG_SPC_GRY',  4, 1, 'PRODUCT', 'TXFX_DH_150SD',     0.500, 'HT 150 yarn'),
    ('TXFX_FG_SPC_GRY',  4, 1, 'PRODUCT', 'TXFX_DL_150SD',     0.500, 'LT 150 yarn'),
    -- DTY 75 BR branch -- single preset, no HT/LT split here (matches 000239 simplification)
    ('TXFX_FG_SPC_GRY',  4, 2, 'PRODUCT', 'TXFX_DP_075BR',     1.020, 'Preset 75 BR input'),
    ('TXFX_FG_SPC_GRY',  5, 1, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_SPC_GRY',  5, 2, 'PRODUCT', 'TXFX_DP_150SD',     1.000, 'Preset 150 input'),
    ('TXFX_FG_SPC_GRY',  6, 1, 'PRODUCT', 'TXFX_P_150SD',      1.020, 'POY 150 SD input'),
    ('TXFX_FG_SPC_GRY',  6, 2, 'PRODUCT', 'TXFX_P_075BR',      1.020, 'POY 75 BR input'),
    ('TXFX_FG_SPC_GRY',  7, 1, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_SPC_GRY',  7, 2, 'PRODUCT', 'TXFX_PU_BR',        1.000, 'Undrawn BR input'),
    ('TXFX_FG_SPC_GRY',  8, 1, 'PRODUCT', 'TXFX_C3_SD',        1.000, 'Dried SD chip input'),
    ('TXFX_FG_SPC_GRY',  8, 2, 'PRODUCT', 'TXFX_C3_BR',        1.000, 'Dried BR chip input'),
    ('TXFX_FG_SPC_GRY',  9, 1, 'PRODUCT', 'TXFX_C2_SD',        1.000, 'Blended SD chip input'),
    ('TXFX_FG_SPC_GRY',  9, 2, 'PRODUCT', 'TXFX_C2_BR',        1.000, 'Blended BR chip input'),
    ('TXFX_FG_SPC_GRY', 10, 1, 'PRODUCT', 'TXFX_C1_SD',        1.000, 'Raw SD chip input'),
    ('TXFX_FG_SPC_GRY', 10, 2, 'PRODUCT', 'TXFX_C1_BR',        1.000, 'Raw BR chip input'),
    ('TXFX_FG_SPC_GRY', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1 SD'),
    ('TXFX_FG_SPC_GRY', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2 SD'),
    ('TXFX_FG_SPC_GRY', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 SD'),
    ('TXFX_FG_SPC_GRY', 11, 2, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1 BR'),
    ('TXFX_FG_SPC_GRY', 11, 2, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2 BR');

-- ============== TXFX_FG_TRC_RED ==============
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_TRC_RED',  1, 1, 'PRODUCT', 'TXFX_DY_TRC_RED',   1.020, 'Dyed tricot input'),
    ('TXFX_FG_TRC_RED',  2, 1, 'PRODUCT', 'TXFX_G_TRC_120',    1.050, 'Greige tricot input'),
    ('TXFX_FG_TRC_RED',  2, 1, 'ITEM',    '202007193',         0.030, 'Red dyestuff'),
    ('TXFX_FG_TRC_RED',  2, 1, 'ITEM',    '202411819',         0.008, 'Dye auxiliary'),
    ('TXFX_FG_TRC_RED',  3, 1, 'PRODUCT', 'TXFX_DTY_075SD_I',  1.030, 'DTY 75 IM input (linear)'),
    ('TXFX_FG_TRC_RED',  4, 1, 'PRODUCT', 'TXFX_DH_075SD',     0.510, 'HT 75 yarn'),
    ('TXFX_FG_TRC_RED',  4, 1, 'PRODUCT', 'TXFX_DL_075SD',     0.510, 'LT 75 yarn'),
    ('TXFX_FG_TRC_RED',  5, 1, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_TRC_RED',  5, 2, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_TRC_RED',  6, 1, 'PRODUCT', 'TXFX_P_075SD',      1.020, 'POY 75 input'),
    ('TXFX_FG_TRC_RED',  7, 1, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_TRC_RED',  8, 1, 'PRODUCT', 'TXFX_C3_SD',        1.000, 'Dried chip input'),
    ('TXFX_FG_TRC_RED',  9, 1, 'PRODUCT', 'TXFX_C2_SD',        1.000, 'Blended chip input'),
    ('TXFX_FG_TRC_RED', 10, 1, 'PRODUCT', 'TXFX_C1_SD',        0.990, 'Raw chip input'),
    ('TXFX_FG_TRC_RED', 10, 1, 'PRODUCT', 'TXFX_C1_MB_BLK',    0.020, 'Black masterbatch (red overdyed)'),
    ('TXFX_FG_TRC_RED', 10, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    ('TXFX_FG_TRC_RED', 11, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1'),
    ('TXFX_FG_TRC_RED', 11, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_TRC_RED', 11, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in'),
    ('TXFX_FG_TRC_RED', 11, 2, 'ITEM',    '202503831',         0.500, 'Black pigment'),
    ('TXFX_FG_TRC_RED', 11, 2, 'ITEM',    '202303677',         0.500, 'MB carrier resin');

-- ============== TXFX_FG_DTY075I_CN ==============  Cone yarn
INSERT INTO _dag_rms VALUES
    ('TXFX_FG_DTY075I_CN', 1, 1, 'PRODUCT', 'TXFX_DTY_075SD_I',  1.005, 'DTY 75 IM input'),
    ('TXFX_FG_DTY075I_CN', 1, 1, 'ITEM',    '202303677',         0.020, 'Cone packing additive'),
    ('TXFX_FG_DTY075I_CN', 2, 1, 'PRODUCT', 'TXFX_DH_075SD',     0.510, 'HT 75 yarn'),
    ('TXFX_FG_DTY075I_CN', 2, 1, 'PRODUCT', 'TXFX_DL_075SD',     0.510, 'LT 75 yarn'),
    ('TXFX_FG_DTY075I_CN', 3, 1, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_DTY075I_CN', 3, 2, 'PRODUCT', 'TXFX_DP_075SD',     1.000, 'Preset 75 input'),
    ('TXFX_FG_DTY075I_CN', 4, 1, 'PRODUCT', 'TXFX_P_075SD',      1.020, 'POY 75 input'),
    ('TXFX_FG_DTY075I_CN', 5, 1, 'PRODUCT', 'TXFX_PU_SD',        1.000, 'Undrawn SD input'),
    ('TXFX_FG_DTY075I_CN', 6, 1, 'PRODUCT', 'TXFX_C3_SD',        1.000, 'Dried chip input'),
    ('TXFX_FG_DTY075I_CN', 7, 1, 'PRODUCT', 'TXFX_C2_SD',        1.000, 'Blended chip input'),
    ('TXFX_FG_DTY075I_CN', 8, 1, 'PRODUCT', 'TXFX_C1_SD',        0.990, 'Raw chip input'),
    ('TXFX_FG_DTY075I_CN', 8, 1, 'PRODUCT', 'TXFX_C1_MB_BLK',    0.020, 'Black masterbatch'),
    ('TXFX_FG_DTY075I_CN', 8, 1, 'ITEM',    '202303677',         0.005, 'Blend additive'),
    ('TXFX_FG_DTY075I_CN', 9, 1, 'ITEM',    '202303677',         0.860, 'Polymer feedstock 1'),
    ('TXFX_FG_DTY075I_CN', 9, 1, 'ITEM',    '202108613',         0.350, 'Polymer feedstock 2'),
    ('TXFX_FG_DTY075I_CN', 9, 1, 'ITEM',    '202411819',         0.005, 'TiO2 / spin oil stand-in'),
    ('TXFX_FG_DTY075I_CN', 9, 2, 'ITEM',    '202503831',         0.500, 'Black pigment'),
    ('TXFX_FG_DTY075I_CN', 9, 2, 'ITEM',    '202303677',         0.500, 'MB carrier resin');

-- =============================================================================
-- 3. Resolve sys_ids + per-level seq counts (for x positioning).
-- =============================================================================
CREATE TEMP TABLE _dag_level_counts AS
SELECT fg_code, lvl, COUNT(*) AS slot_count
  FROM _dag_seqs
 GROUP BY fg_code, lvl;

-- =============================================================================
-- 4. Build cost_route_head + cost_route_seq + cost_route_rm via PL/pgSQL loop.
-- =============================================================================
DO $$
DECLARE
    fg              RECORD;
    s               RECORD;
    rm              RECORD;
    v_fg_sys_id     BIGINT;
    v_head_id       BIGINT;
    v_seq_id        BIGINT;
    v_seq_product_id BIGINT;
    v_rm_product_id BIGINT;
    v_target_seq_id BIGINT;
    v_target_product_id BIGINT;
    v_slot_count    INT;
    v_x             NUMERIC(10,2);
    v_y             NUMERIC(10,2);
BEGIN
    FOR fg IN
        SELECT DISTINCT fg_code FROM _dag_seqs ORDER BY fg_code
    LOOP
        SELECT sys_id INTO v_fg_sys_id FROM _txfx_dag_products WHERE code = fg.fg_code;
        IF v_fg_sys_id IS NULL THEN
            RAISE EXCEPTION '000244: FG product code % not found in cost_product_master', fg.fg_code;
        END IF;

        -- Idempotent: skip if a non-LOCKED head already exists.
        SELECT crh_head_id INTO v_head_id
          FROM cost_route_head
         WHERE crh_product_sys_id = v_fg_sys_id
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
            v_fg_sys_id, 'COMPLETE', 1,
            'Self-contained multi-product DAG for ' || fg.fg_code || ' (seed 000244)',
            'seed_000244', 'seed_000244'
        ) RETURNING crh_head_id INTO v_head_id;

        -- Insert all seqs for this FG.
        FOR s IN
            SELECT ds.lvl, ds.seq, ds.product_code, ds.route_name, lc.slot_count
              FROM _dag_seqs ds
              JOIN _dag_level_counts lc ON lc.fg_code = ds.fg_code AND lc.lvl = ds.lvl
             WHERE ds.fg_code = fg.fg_code
             ORDER BY ds.lvl, ds.seq
        LOOP
            SELECT sys_id INTO v_seq_product_id FROM _txfx_dag_products WHERE code = s.product_code;
            IF v_seq_product_id IS NULL THEN
                RAISE EXCEPTION '000244: seq product code % (fg=%) not found',
                                s.product_code, fg.fg_code;
            END IF;

            -- Positions:
            --   y = lvl * 180 (FG at top, RM at bottom)
            --   x = (seq - 0.5*(N+1)) * 300 + 600  ;; centered around x=600
            v_y := s.lvl * 180;
            v_slot_count := s.slot_count;
            v_x := (s.seq::NUMERIC - (v_slot_count + 1)::NUMERIC / 2.0) * 300.0 + 600.0;

            INSERT INTO cost_route_seq (
                crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
                crs_route_name, crs_position_x, crs_position_y,
                crs_created_by, crs_updated_by
            ) VALUES (
                v_head_id, v_seq_product_id, s.lvl, s.seq,
                s.route_name, v_x, v_y,
                'seed_000244', 'seed_000244'
            );
        END LOOP;

        -- Insert all RMs for this FG.
        FOR rm IN
            SELECT t_lvl, t_seq, rm_type, rm_ref, ratio, rm_name
              FROM _dag_rms
             WHERE fg_code = fg.fg_code
             ORDER BY t_lvl, t_seq
        LOOP
            -- Find target seq id.
            SELECT crs.crs_seq_id, crs.crs_product_sys_id
              INTO v_target_seq_id, v_target_product_id
              FROM cost_route_seq crs
             WHERE crs.crs_head_id = v_head_id
               AND crs.crs_route_level = rm.t_lvl
               AND crs.crs_route_seq = rm.t_seq;
            IF v_target_seq_id IS NULL THEN
                RAISE EXCEPTION '000244: target seq (lvl=%, seq=%) missing for fg=%',
                                rm.t_lvl, rm.t_seq, fg.fg_code;
            END IF;

            IF rm.rm_type = 'PRODUCT' THEN
                SELECT sys_id INTO v_rm_product_id FROM _txfx_dag_products WHERE code = rm.rm_ref;
                IF v_rm_product_id IS NULL THEN
                    RAISE EXCEPTION '000244: PRODUCT rm code % (fg=%) not found in master',
                                    rm.rm_ref, fg.fg_code;
                END IF;
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_product_sys_id, crm_rm_type, crm_route_rm_name,
                    crm_route_rm_ratio, crm_created_by, crm_updated_by
                ) VALUES (
                    v_target_seq_id, v_target_product_id,
                    v_rm_product_id, 'PRODUCT', rm.rm_name,
                    rm.ratio, 'seed_000244', 'seed_000244'
                );
            ELSIF rm.rm_type = 'ITEM' THEN
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_item_code, crm_rm_type, crm_route_rm_name, crm_route_rm_item_code,
                    crm_route_rm_ratio, crm_created_by, crm_updated_by
                ) VALUES (
                    v_target_seq_id, v_target_product_id,
                    rm.rm_ref, 'ITEM', rm.rm_name, rm.rm_ref,
                    rm.ratio, 'seed_000244', 'seed_000244'
                );
            ELSE
                RAISE EXCEPTION '000244: unsupported rm_type % for fg=%', rm.rm_type, fg.fg_code;
            END IF;
        END LOOP;
    END LOOP;
END $$;

DROP TABLE _dag_level_counts;
DROP TABLE _dag_rms;
DROP TABLE _dag_seqs;
DROP TABLE _txfx_dag_products;

COMMIT;
