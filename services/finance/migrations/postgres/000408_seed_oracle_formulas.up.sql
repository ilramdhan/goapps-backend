-- 000408_seed_oracle_formulas.up.sql
-- Seeds 77 formulas (64 active + 13 pending).
-- LOOKUP (29) and INTERMINGLING (1) types are skipped — handled by fill-group at form-fill time.
--
-- NOTE: F_YARN_OIL_GAIN and F_YARN_OIL_GAIN_ZERO both target OIL_GAIN as result_param_id.
-- The unique index on (result_param_id) WHERE deleted_at IS NULL prevents two active formulas
-- sharing the same result param. F_YARN_OIL_GAIN_ZERO is therefore seeded as is_active=FALSE
-- (it is a semantically identical zero-constant alias; the active F_YARN_OIL_GAIN takes precedence).

-- ============================================================
-- PART 1: Insert formulas (active=TRUE unless PENDING or duplicate result_param)
-- Rows with unresolvable result_param_id are skipped automatically (subquery returns NULL
-- which violates the NOT NULL constraint, causing the row to be excluded by the CTE).
-- ============================================================

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, version, is_active, created_at, created_by)
SELECT f.code, f.name, f.ftype, f.expr,
       (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1),
       f.descr, 1, f.active, NOW(), 'seed_formula_oracle'
FROM (VALUES
-- CALCULATION formulas (52 active)
  ('F_YARN_A9_WT','A9 Weight','CALCULATION','AX_WT * A9_PERC / AX_PERC','A9_WT','Grade weight A9',TRUE),
  ('F_YARN_AE_WT','AE Weight','CALCULATION','AX_WT * AE_PERC / AX_PERC','AE_WT','Grade weight AE',TRUE),
  ('F_YARN_A_WT','A Weight','CALCULATION','AX_WT * A_PERC / AX_PERC','A_WT','Grade weight A',TRUE),
  ('F_YARN_B_WT','B Weight','CALCULATION','AX_WT * B_PERC / AX_PERC','B_WT','Grade weight B',TRUE),
  ('F_YARN_C_WT','C Weight','CALCULATION','AX_WT * C_PERC / AX_PERC','C_WT','Grade weight C',TRUE),
  ('F_YARN_NET_BOB_WT','Net Bobbin Weight','CALCULATION','AX_WT + AE_WT + A9_WT + A_WT + B_WT + C_WT','NET_BOB_WT','Sum of grade weights',TRUE),
  ('F_YARN_BATCH_WEIGHT','Batch Weight','CALCULATION','NO_OF_TROLLIES * NO_BOB_PER_TROLLIES * NET_BOB_WT','BATCH_WEIGHT','Total batch weight',TRUE),
  ('F_YARN_NET_PROD','Net Production','CALCULATION','NO_OF_POSITION * MC_SPEED * (MC_EFFICIENCY / 100.0) * 1440.0 * DENIER / 9000000.0','NET_PRODUCTION','kg/day output',TRUE),
  ('F_YARN_RM_NORMS','RM Norms','CALCULATION','1.0 / (1.0 - WASTE_PERC / 100.0)','RM_NORMS','Raw material normalisation factor',TRUE),
  ('F_YARN_RM_LANDED','RM Landed Cost','CALCULATION','RM_RATE','RM_LANDED_COST','Pass-through from RM_RATE',TRUE),
  ('F_YARN_OIL_COST','Oil Cost','CALCULATION','OIL_RATE * OPU / 100.0','OIL_COST','Oil cost per kg',TRUE),
  ('F_YARN_OIL_GAIN','Oil Gain','CALCULATION','0','OIL_GAIN','Always 0',TRUE),
  ('F_YARN_POWER_KG','Power Per Kg','CALCULATION','NET_PRODUCTION > 0 ? POWER_PER_DAY / NET_PRODUCTION : 0','POWER_PER_KG','Power cost per kg',TRUE),
  ('F_YARN_MANPOWER_KG','Manpower Per Kg','CALCULATION','NET_PRODUCTION > 0 ? MANPOWER_PER_DAY / NET_PRODUCTION : 0','MANPOWER_PER_KG','Manpower cost per kg',TRUE),
  ('F_YARN_OVERHEAD_KG','Overhead Per Kg','CALCULATION','NET_PRODUCTION > 0 ? OVERHEAD_PER_HEAD * NO_OF_END / NET_PRODUCTION : 0','OVERHEAD_PER_KG','Overhead per kg',TRUE),
  ('F_YARN_SPARES_KG','Spares Per Kg','CALCULATION','NET_PRODUCTION > 0 ? SPARESCOST_PER_DAY / NET_PRODUCTION : 0','SPARESCOST_PER_KG','Spares cost per kg',TRUE),
  ('F_YARN_TOTAL_FIXED','Total Fixed Cost Per Kg','CALCULATION','POWER_PER_KG + MANPOWER_PER_KG + OVERHEAD_PER_KG + SPARESCOST_PER_KG','TOTAL_FIXEDCOST_PER_KG','Sum of fixed costs per kg',TRUE),
  ('F_YARN_MB_COST','MB Cost Marketing','CALCULATION','MB_RATE_MKT * MB_SP_DOZING / 100.0','MB_COST_MKT','MB cost per kg',TRUE),
  ('F_YARN_HEATSET_KG','Heatset Cost Per Kg','CALCULATION','BATCH_WEIGHT > 0 ? HEATSET_COST_PER_BATCH / BATCH_WEIGHT : 0','HEATSET_COST_PER_KG','Heatset cost per kg',TRUE),
  ('F_YARN_CAP_BOX_WT','Captive Box Weight','CALCULATION','CAPTIVE_NO_OF_BOB * NET_BOB_WT * RM_NORMS','CAPTIVE_BOX_WT','Captive box weight',TRUE),
  ('F_YARN_DEL_BOX_WT','Delivery Box Weight','CALCULATION','DELIVERY_NO_OF_BOB * NET_BOB_WT * RM_NORMS','DELIVERY_BOX_WT','Delivery box weight',TRUE),
  ('F_YARN_CAP_PACK','Captive Pack Cost','CALCULATION','CAPTIVE_BOX_WT > 0 ? (CAPTIVE_NO_OF_BOB * CAPTIVE_BOB_RATE + CAPTIVE_BOX_RATE) / CAPTIVE_BOX_WT : 0','CAPTIVE_PACK_COST','Packing cost captive',TRUE),
  ('F_YARN_DEL_PACK','Delivery Pack Cost','CALCULATION','DELIVERY_BOX_WT > 0 ? (DELIVERY_NO_OF_BOB * DELIVERY_BOB_RATE + DELIVERY_BOX_RATE) / DELIVERY_BOX_WT : 0','DELIVERY_PACK_COST','Packing cost delivery',TRUE),
  ('F_YARN_CONV_CAP','Conversion Captive','CALCULATION','TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1','ONLY_CONV_CAP_PACK_EXCL_MB','Captive conversion excluding MB',TRUE),
  ('F_YARN_CONV_DEL','Conversion Delivery','CALCULATION','TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1','ONLY_CONV_DEL_PACK_EXCL_MB','Delivery conversion excluding MB',TRUE),
  ('F_YARN_CAP_PRE_QL','Captive Cost Before Quality Loss','CALCULATION','RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB','CAPTIVE_COST_BEFORE_QLOSS','Captive pre-quality-loss cost',TRUE),
  ('F_YARN_DEL_PRE_QL','Delivery Cost Before Quality Loss','CALCULATION','RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB','DELIVERY_COST_BEFORE_QLOSS','Delivery pre-quality-loss cost',TRUE),
  ('F_YARN_BC_LOSS_CAP','BC Value Loss Captive','CALCULATION','CAPTIVE_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)','BC_VAL_LOSS_CAPTIVE','BC quality loss captive',TRUE),
  ('F_YARN_BC_LOSS_DEL','BC Value Loss Delivery','CALCULATION','DELIVERY_COST_BEFORE_QLOSS * (BC_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)','BC_VAL_LOSS_DELIVERY','BC quality loss delivery',TRUE),
  ('F_YARN_NON_STD_LOSS','Non-Standard Value Loss','CALCULATION','CAPTIVE_COST_BEFORE_QLOSS * (NON_STD_SPECIAL_PROD / 100.0) * (1.0 - VALUE_LOSS / 100.0)','NON_STD_VALUE_LOSS','Non-std quality loss',TRUE),
  ('F_YARN_QLOSS_CAP','Quality Loss Captive','CALCULATION','BC_VAL_LOSS_CAPTIVE + NON_STD_VALUE_LOSS','QLTY_LOSS_CAPTIVE_COST','Total quality loss captive',TRUE),
  ('F_YARN_QLOSS_DEL','Quality Loss Delivery','CALCULATION','BC_VAL_LOSS_DELIVERY + NON_STD_VALUE_LOSS','QLTY_LOSS_DELIVERY_COST','Total quality loss delivery',TRUE),
  ('F_YARN_CAP_FINAL','Captive Final Cost','CALCULATION','CAPTIVE_COST_BEFORE_QLOSS + QLTY_LOSS_CAPTIVE_COST','CAPTIVE_COST_QLTY_LOSS','Captive cost with quality loss',TRUE),
  ('F_YARN_DEL_FINAL','Delivery Final Cost','CALCULATION','DELIVERY_COST_BEFORE_QLOSS + QLTY_LOSS_DELIVERY_COST','DELIVERY_COST_QLTY_LOSS','Delivery cost with quality loss',TRUE),
  ('F_YARN_VB1_LOSS','VB1 Loss','CALCULATION','VOLUME_BUCKET_1_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_1_QTY : 0','VOLUME_BUCKET_1_LOSS','Change-over loss per unit VB1',TRUE),
  ('F_YARN_VB2_LOSS','VB2 Loss','CALCULATION','VOLUME_BUCKET_2_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_2_QTY : 0','VOLUME_BUCKET_2_LOSS','Change-over loss per unit VB2',TRUE),
  ('F_YARN_VB3_LOSS','VB3 Loss','CALCULATION','VOLUME_BUCKET_3_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_3_QTY : 0','VOLUME_BUCKET_3_LOSS','Change-over loss per unit VB3',TRUE),
  ('F_YARN_VB4_LOSS','VB4 Loss','CALCULATION','VOLUME_BUCKET_4_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_4_QTY : 0','VOLUME_BUCKET_4_LOSS','Change-over loss per unit VB4',TRUE),
  ('F_YARN_VB5_LOSS','VB5 Loss','CALCULATION','VOLUME_BUCKET_5_QTY > 0 ? CHANGE_OVER_QLTY_LOSS / VOLUME_BUCKET_5_QTY : 0','VOLUME_BUCKET_5_LOSS','Change-over loss per unit VB5',TRUE),
  ('F_YARN_VB1_DEL','VB1 Delivery Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_1_LOSS','VOLUME_BUCKET_1_DEL_COST','Delivery cost VB1',TRUE),
  ('F_YARN_VB2_DEL','VB2 Delivery Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_2_LOSS','VOLUME_BUCKET_2_DEL_COST','Delivery cost VB2',TRUE),
  ('F_YARN_VB3_DEL','VB3 Delivery Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_3_LOSS','VOLUME_BUCKET_3_DEL_COST','Delivery cost VB3',TRUE),
  ('F_YARN_VB4_DEL','VB4 Delivery Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_4_LOSS','VOLUME_BUCKET_4_DEL_COST','Delivery cost VB4',TRUE),
  ('F_YARN_VB5_DEL','VB5 Delivery Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + VOLUME_BUCKET_5_LOSS','VOLUME_BUCKET_5_DEL_COST','Delivery cost VB5',TRUE),
  ('F_YARN_WASHING_COST','Washing Cost','CALCULATION','0','WASHING_COST','Always 0',TRUE),
  ('F_YARN_STEAM_COST','Steam Cost CNG','CALCULATION','0','STEAM_COST_CNG','Always 0',TRUE),
  -- F_YARN_OIL_GAIN_ZERO shares OIL_GAIN result_param_id with F_YARN_OIL_GAIN (active).
  -- Seeded as is_active=FALSE to satisfy the unique index on (result_param_id) WHERE deleted_at IS NULL.
  ('F_YARN_OIL_GAIN_ZERO','Oil Gain Zero','CALCULATION','0','OIL_GAIN','Always 0 (oil gain not costed)',FALSE),
  ('F_YARN_CONV_FACTOR','Conversion Factor','CALCULATION','1','CONV_FACTOR','Always 1 (no conversion)',TRUE),
  ('F_YARN_RP_CC','RP-CC','CALCULATION','CROSS_SECTION','RP_CC','Copy cross section to RP-CC',TRUE),
  ('F_YARN_SPECIAL_COST_2','Special Cost 2','CALCULATION','SPECIAL_COST_1','SPECIAL_COST_2','Mirror of special cost 1',TRUE),
  ('F_YARN_SPECIAL_COST_FLAG_PASS','Special Cost Flag','CALCULATION','SPECIAL_COST_FLAG','SPECIAL_COST_FLAG','Pass-through',TRUE),
  ('F_YARN_WASTE_LESS_MB_OPU','Waste Less MB OPU','CALCULATION','(1.0 - WASTE_PERC / 100.0) - (MB_SP_DOZING / 100.0) - (OPU / 100.0)','WASTE_LESS_MB_OPU','Effective yield factor',TRUE),
-- CONDITIONAL formulas (3 active)
  ('F_YARN_HEATSET_KG_COND','Heatset Cost Per Kg (conditional)','CONDITIONAL','BATCH_WEIGHT > 0 ? HEATSET_COST_PER_BATCH / BATCH_WEIGHT : 0','HEATSET_COST_PER_KG','Guard against zero batch weight',TRUE),
  ('F_YARN_MB_FLAG','MB Flag','CONDITIONAL','MB_SP_DOZING > 0 ? ''Y'' : ''N''','MB_FLAG','Y if masterbatch present',TRUE),
  ('F_YARN_RP_DOZING','RP-Dozing','CONDITIONAL','MB_SP_DOZING > 0 ? MB_SP_DOZING : 0','RP_DOZING','Dozing value or zero',TRUE),
-- RM_LOOKUP formulas (3 active)
  ('F_YARN_RM_RATE','RM Rate','RM_LOOKUP','sum(ratio * CASE rm_type WHEN ''GROUP'' THEN mst_rm_cost(rm_group_code,period,pricing_type) WHEN ''PRODUCT'' THEN upstream_product(rm_product_legacy_id).COST_CAP_FINAL END) for route_rms WHERE route_head_legacy_product_id=current_product AND route_level=current_level','RM_RATE','Raw material rate from route',TRUE),
  ('F_YARN_CAP_CONVERSION','Captive Conversion','RM_LOOKUP','sum(ratio * CASE rm_type WHEN ''PRODUCT'' THEN upstream_product(rm_product_legacy_id).COST_CAP_FINAL WHEN ''GROUP'' THEN mst_rm_cost(rm_group_code,period,''VAL'') END) for route_rms WHERE route_head_legacy_product_id=current_product AND route_level=current_level','CAPTIVE_CONVERSION','Captive conversion from routing',TRUE),
  ('F_YARN_DEL_CONVERSION','Delivery Conversion','RM_LOOKUP','sum(ratio * CASE rm_type WHEN ''PRODUCT'' THEN upstream_product(rm_product_legacy_id).COST_DEL_FINAL WHEN ''GROUP'' THEN mst_rm_cost(rm_group_code,period,''VAL'') END) for route_rms WHERE route_head_legacy_product_id=current_product AND route_level=current_level','DELIVERY_CONVERSION','Delivery conversion from routing',TRUE),
-- SNAPSHOT formula (1 active)
  ('F_YARN_DEL_QLOSS_PRE_PROC','Delivery Cost Snapshot','SNAPSHOT','snapshot(DELIVERY_COST_BEFORE_QLOSS) at process start','DELIVERY_COST_BEFORE_QLOSS_BEFORE_PROCESS','Audit checkpoint before process adjustment',TRUE),
-- FROM_MARKETING formulas (5 active)
  ('F_YARN_AX_WT_FROM_MKT','AX_WT from Marketing','FROM_MARKETING','marketing_result(product,''AX_WT'',period)','AX_WT','Copy AX_WT from MKT session — 100% match in Oracle data',TRUE),
  ('F_YARN_CAP_NO_BOB_FROM_MKT','Captive No of Bob from MKT','FROM_MARKETING','marketing_result(product,''CAPTIVE_NO_OF_BOB'',period)','CAPTIVE_NO_OF_BOB','Copy CAPTIVE_NO_OF_BOB from MKT — 100% match',TRUE),
  ('F_YARN_DEL_NO_BOB_FROM_MKT','Delivery No of Bob from MKT','FROM_MARKETING','marketing_result(product,''DELIVERY_NO_OF_BOB'',period)','DELIVERY_NO_OF_BOB','Copy DELIVERY_NO_OF_BOB from MKT — 100% match',TRUE),
  ('F_YARN_HEATSET_CODE_FROM_MKT','Heatset Code from MKT','FROM_MARKETING','marketing_result(product,''HEATSET_CODE'',period)','HEATSET_CODE','Copy HEATSET_CODE from MKT — 100% match',TRUE),
  ('F_YARN_DOZING_ADJ_FROM_MKT','Dozing Adjust from MKT','FROM_MARKETING','marketing_result(product,''DOZING_ADJUST'',period)','DOZING_ADJUST','Copy DOZING_ADJUST from MKT — 97% match; VAL always mirrors MKT per IT Lead decision',TRUE),
-- PENDING formulas (13 inactive)
  ('F_YARN_ADDITIONAL_VAL_LOSS','Additional Value Loss','PENDING','TBD','ADDITIONAL_VAL_LOSS','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_ADD_NON_STD_BC_LOSS','Additional Non-Standard BC Loss','PENDING','TBD','ADD_NON_STD_BC_LOSS','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_BC_SP','BC Special Product','PENDING','TBD','BC_SP','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_DEL_QLOSS_ADDITION','Delivery QLoss Addition','PENDING','TBD: DELIVERY_COST_BEFORE_QLOSS * DELIVERY_COST_BEFORE_QLOSS_ADD_PER / 100','DELIVERY_COST_BEFORE_QLOSS_ADDITION','Partial definition, awaiting costing team',FALSE),
  ('F_YARN_DEL_QLOSS_ADD_PER','Delivery QLoss Add Per','PENDING','TBD (user input)','DELIVERY_COST_BEFORE_QLOSS_ADD_PER','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_NON_STD_BC_SP','Non-Std BC SP','PENDING','TBD','NON_STD_BC_SP','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_AE_A9_A','R AE/A9/A','PENDING','TBD','R_AE_A9_A','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_AX','R AX','PENDING','TBD','R_AX','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_BC','R BC','PENDING','TBD','R_BC','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_BC_LOSS','R BC Loss','PENDING','TBD','R_BC_LOSS','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_NON_STD_DIFF','R Non-Std Diff','PENDING','TBD','R_NON_STD_DIFF','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_NON_STD_LOSS','R Non-Std Loss','PENDING','TBD','R_NON_STD_LOSS','Not in Oracle pkg_yarn_calculation',FALSE),
  ('F_YARN_R_NON_STD_SP','R Non-Std SP','PENDING','TBD','R_NON_STD_SP','Not in Oracle pkg_yarn_calculation',FALSE)
) AS f(code, name, ftype, expr, result_code, descr, active)
WHERE NOT EXISTS (
    SELECT 1 FROM mst_formula WHERE formula_code = f.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Insert formula_param (input param links)
-- One row per input param per formula.
-- Uses param_code lookup; skips if param or formula not found.
-- NOT EXISTS guard makes this idempotent.
-- ============================================================

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT
    (SELECT id FROM mst_formula WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1),
    (SELECT id FROM mst_parameter WHERE param_code = fp.pcode AND deleted_at IS NULL LIMIT 1),
    fp.sort_order
FROM (VALUES
-- F_YARN_A9_WT: AX_WT, A9_PERC, AX_PERC
  ('F_YARN_A9_WT','AX_WT',1),('F_YARN_A9_WT','A9_PERC',2),('F_YARN_A9_WT','AX_PERC',3),
-- F_YARN_AE_WT: AX_WT, AE_PERC, AX_PERC
  ('F_YARN_AE_WT','AX_WT',1),('F_YARN_AE_WT','AE_PERC',2),('F_YARN_AE_WT','AX_PERC',3),
-- F_YARN_A_WT: AX_WT, A_PERC, AX_PERC
  ('F_YARN_A_WT','AX_WT',1),('F_YARN_A_WT','A_PERC',2),('F_YARN_A_WT','AX_PERC',3),
-- F_YARN_B_WT: AX_WT, B_PERC, AX_PERC
  ('F_YARN_B_WT','AX_WT',1),('F_YARN_B_WT','B_PERC',2),('F_YARN_B_WT','AX_PERC',3),
-- F_YARN_C_WT: AX_WT, C_PERC, AX_PERC
  ('F_YARN_C_WT','AX_WT',1),('F_YARN_C_WT','C_PERC',2),('F_YARN_C_WT','AX_PERC',3),
-- F_YARN_NET_BOB_WT: AX_WT, AE_WT, A9_WT, A_WT, B_WT, C_WT
  ('F_YARN_NET_BOB_WT','AX_WT',1),('F_YARN_NET_BOB_WT','AE_WT',2),('F_YARN_NET_BOB_WT','A9_WT',3),
  ('F_YARN_NET_BOB_WT','A_WT',4),('F_YARN_NET_BOB_WT','B_WT',5),('F_YARN_NET_BOB_WT','C_WT',6),
-- F_YARN_BATCH_WEIGHT: NO_OF_TROLLIES, NO_BOB_PER_TROLLIES, NET_BOB_WT
  ('F_YARN_BATCH_WEIGHT','NO_OF_TROLLIES',1),('F_YARN_BATCH_WEIGHT','NO_BOB_PER_TROLLIES',2),('F_YARN_BATCH_WEIGHT','NET_BOB_WT',3),
-- F_YARN_NET_PROD: NO_OF_POSITION, MC_SPEED, MC_EFFICIENCY, DENIER
  ('F_YARN_NET_PROD','NO_OF_POSITION',1),('F_YARN_NET_PROD','MC_SPEED',2),('F_YARN_NET_PROD','MC_EFFICIENCY',3),('F_YARN_NET_PROD','DENIER',4),
-- F_YARN_RM_NORMS: WASTE_PERC
  ('F_YARN_RM_NORMS','WASTE_PERC',1),
-- F_YARN_RM_LANDED: RM_RATE
  ('F_YARN_RM_LANDED','RM_RATE',1),
-- F_YARN_OIL_COST: OIL_RATE, OPU
  ('F_YARN_OIL_COST','OIL_RATE',1),('F_YARN_OIL_COST','OPU',2),
-- F_YARN_POWER_KG: POWER_PER_DAY, NET_PRODUCTION
  ('F_YARN_POWER_KG','POWER_PER_DAY',1),('F_YARN_POWER_KG','NET_PRODUCTION',2),
-- F_YARN_MANPOWER_KG: MANPOWER_PER_DAY, NET_PRODUCTION
  ('F_YARN_MANPOWER_KG','MANPOWER_PER_DAY',1),('F_YARN_MANPOWER_KG','NET_PRODUCTION',2),
-- F_YARN_OVERHEAD_KG: OVERHEAD_PER_HEAD, NO_OF_END, NET_PRODUCTION
  ('F_YARN_OVERHEAD_KG','OVERHEAD_PER_HEAD',1),('F_YARN_OVERHEAD_KG','NO_OF_END',2),('F_YARN_OVERHEAD_KG','NET_PRODUCTION',3),
-- F_YARN_SPARES_KG: SPARESCOST_PER_DAY, NET_PRODUCTION
  ('F_YARN_SPARES_KG','SPARESCOST_PER_DAY',1),('F_YARN_SPARES_KG','NET_PRODUCTION',2),
-- F_YARN_TOTAL_FIXED: POWER_PER_KG, MANPOWER_PER_KG, OVERHEAD_PER_KG, SPARESCOST_PER_KG
  ('F_YARN_TOTAL_FIXED','POWER_PER_KG',1),('F_YARN_TOTAL_FIXED','MANPOWER_PER_KG',2),('F_YARN_TOTAL_FIXED','OVERHEAD_PER_KG',3),('F_YARN_TOTAL_FIXED','SPARESCOST_PER_KG',4),
-- F_YARN_MB_COST: MB_RATE_MKT, MB_SP_DOZING
  ('F_YARN_MB_COST','MB_RATE_MKT',1),('F_YARN_MB_COST','MB_SP_DOZING',2),
-- F_YARN_HEATSET_KG: HEATSET_COST_PER_BATCH, BATCH_WEIGHT
  ('F_YARN_HEATSET_KG','HEATSET_COST_PER_BATCH',1),('F_YARN_HEATSET_KG','BATCH_WEIGHT',2),
-- F_YARN_CAP_BOX_WT: CAPTIVE_NO_OF_BOB, NET_BOB_WT, RM_NORMS
  ('F_YARN_CAP_BOX_WT','CAPTIVE_NO_OF_BOB',1),('F_YARN_CAP_BOX_WT','NET_BOB_WT',2),('F_YARN_CAP_BOX_WT','RM_NORMS',3),
-- F_YARN_DEL_BOX_WT: DELIVERY_NO_OF_BOB, NET_BOB_WT, RM_NORMS
  ('F_YARN_DEL_BOX_WT','DELIVERY_NO_OF_BOB',1),('F_YARN_DEL_BOX_WT','NET_BOB_WT',2),('F_YARN_DEL_BOX_WT','RM_NORMS',3),
-- F_YARN_CAP_PACK: CAPTIVE_NO_OF_BOB, CAPTIVE_BOB_RATE, CAPTIVE_BOX_RATE, CAPTIVE_BOX_WT
  ('F_YARN_CAP_PACK','CAPTIVE_NO_OF_BOB',1),('F_YARN_CAP_PACK','CAPTIVE_BOB_RATE',2),('F_YARN_CAP_PACK','CAPTIVE_BOX_RATE',3),('F_YARN_CAP_PACK','CAPTIVE_BOX_WT',4),
-- F_YARN_DEL_PACK: DELIVERY_NO_OF_BOB, DELIVERY_BOB_RATE, DELIVERY_BOX_RATE, DELIVERY_BOX_WT
  ('F_YARN_DEL_PACK','DELIVERY_NO_OF_BOB',1),('F_YARN_DEL_PACK','DELIVERY_BOB_RATE',2),('F_YARN_DEL_PACK','DELIVERY_BOX_RATE',3),('F_YARN_DEL_PACK','DELIVERY_BOX_WT',4),
-- F_YARN_CONV_CAP: TOTAL_FIXEDCOST_PER_KG, CAPTIVE_PACK_COST, OIL_COST, INTERMINGLING, SPECIAL_COST_1
  ('F_YARN_CONV_CAP','TOTAL_FIXEDCOST_PER_KG',1),('F_YARN_CONV_CAP','CAPTIVE_PACK_COST',2),('F_YARN_CONV_CAP','OIL_COST',3),('F_YARN_CONV_CAP','INTERMINGLING',4),('F_YARN_CONV_CAP','SPECIAL_COST_1',5),
-- F_YARN_CONV_DEL: TOTAL_FIXEDCOST_PER_KG, DELIVERY_PACK_COST, OIL_COST, INTERMINGLING, SPECIAL_COST_1
  ('F_YARN_CONV_DEL','TOTAL_FIXEDCOST_PER_KG',1),('F_YARN_CONV_DEL','DELIVERY_PACK_COST',2),('F_YARN_CONV_DEL','OIL_COST',3),('F_YARN_CONV_DEL','INTERMINGLING',4),('F_YARN_CONV_DEL','SPECIAL_COST_1',5),
-- F_YARN_CAP_PRE_QL: RM_NORMS, RM_LANDED_COST, ONLY_CONV_CAP_PACK_EXCL_MB
  ('F_YARN_CAP_PRE_QL','RM_NORMS',1),('F_YARN_CAP_PRE_QL','RM_LANDED_COST',2),('F_YARN_CAP_PRE_QL','ONLY_CONV_CAP_PACK_EXCL_MB',3),
-- F_YARN_DEL_PRE_QL: RM_NORMS, RM_LANDED_COST, ONLY_CONV_DEL_PACK_EXCL_MB
  ('F_YARN_DEL_PRE_QL','RM_NORMS',1),('F_YARN_DEL_PRE_QL','RM_LANDED_COST',2),('F_YARN_DEL_PRE_QL','ONLY_CONV_DEL_PACK_EXCL_MB',3),
-- F_YARN_BC_LOSS_CAP: CAPTIVE_COST_BEFORE_QLOSS, BC_SPECIAL_PROD, VALUE_LOSS
  ('F_YARN_BC_LOSS_CAP','CAPTIVE_COST_BEFORE_QLOSS',1),('F_YARN_BC_LOSS_CAP','BC_SPECIAL_PROD',2),('F_YARN_BC_LOSS_CAP','VALUE_LOSS',3),
-- F_YARN_BC_LOSS_DEL: DELIVERY_COST_BEFORE_QLOSS, BC_SPECIAL_PROD, VALUE_LOSS
  ('F_YARN_BC_LOSS_DEL','DELIVERY_COST_BEFORE_QLOSS',1),('F_YARN_BC_LOSS_DEL','BC_SPECIAL_PROD',2),('F_YARN_BC_LOSS_DEL','VALUE_LOSS',3),
-- F_YARN_NON_STD_LOSS: CAPTIVE_COST_BEFORE_QLOSS, NON_STD_SPECIAL_PROD, VALUE_LOSS
  ('F_YARN_NON_STD_LOSS','CAPTIVE_COST_BEFORE_QLOSS',1),('F_YARN_NON_STD_LOSS','NON_STD_SPECIAL_PROD',2),('F_YARN_NON_STD_LOSS','VALUE_LOSS',3),
-- F_YARN_QLOSS_CAP: BC_VAL_LOSS_CAPTIVE, NON_STD_VALUE_LOSS
  ('F_YARN_QLOSS_CAP','BC_VAL_LOSS_CAPTIVE',1),('F_YARN_QLOSS_CAP','NON_STD_VALUE_LOSS',2),
-- F_YARN_QLOSS_DEL: BC_VAL_LOSS_DELIVERY, NON_STD_VALUE_LOSS
  ('F_YARN_QLOSS_DEL','BC_VAL_LOSS_DELIVERY',1),('F_YARN_QLOSS_DEL','NON_STD_VALUE_LOSS',2),
-- F_YARN_CAP_FINAL: CAPTIVE_COST_BEFORE_QLOSS, QLTY_LOSS_CAPTIVE_COST
  ('F_YARN_CAP_FINAL','CAPTIVE_COST_BEFORE_QLOSS',1),('F_YARN_CAP_FINAL','QLTY_LOSS_CAPTIVE_COST',2),
-- F_YARN_DEL_FINAL: DELIVERY_COST_BEFORE_QLOSS, QLTY_LOSS_DELIVERY_COST
  ('F_YARN_DEL_FINAL','DELIVERY_COST_BEFORE_QLOSS',1),('F_YARN_DEL_FINAL','QLTY_LOSS_DELIVERY_COST',2),
-- VB loss formulas
  ('F_YARN_VB1_LOSS','CHANGE_OVER_QLTY_LOSS',1),('F_YARN_VB1_LOSS','VOLUME_BUCKET_1_QTY',2),
  ('F_YARN_VB2_LOSS','CHANGE_OVER_QLTY_LOSS',1),('F_YARN_VB2_LOSS','VOLUME_BUCKET_2_QTY',2),
  ('F_YARN_VB3_LOSS','CHANGE_OVER_QLTY_LOSS',1),('F_YARN_VB3_LOSS','VOLUME_BUCKET_3_QTY',2),
  ('F_YARN_VB4_LOSS','CHANGE_OVER_QLTY_LOSS',1),('F_YARN_VB4_LOSS','VOLUME_BUCKET_4_QTY',2),
  ('F_YARN_VB5_LOSS','CHANGE_OVER_QLTY_LOSS',1),('F_YARN_VB5_LOSS','VOLUME_BUCKET_5_QTY',2),
-- VB delivery cost formulas
  ('F_YARN_VB1_DEL','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_VB1_DEL','VOLUME_BUCKET_1_LOSS',2),
  ('F_YARN_VB2_DEL','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_VB2_DEL','VOLUME_BUCKET_2_LOSS',2),
  ('F_YARN_VB3_DEL','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_VB3_DEL','VOLUME_BUCKET_3_LOSS',2),
  ('F_YARN_VB4_DEL','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_VB4_DEL','VOLUME_BUCKET_4_LOSS',2),
  ('F_YARN_VB5_DEL','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_VB5_DEL','VOLUME_BUCKET_5_LOSS',2),
-- F_YARN_RP_CC: CROSS_SECTION
  ('F_YARN_RP_CC','CROSS_SECTION',1),
-- F_YARN_RP_DOZING: MB_SP_DOZING
  ('F_YARN_RP_DOZING','MB_SP_DOZING',1),
-- F_YARN_MB_FLAG: MB_SP_DOZING
  ('F_YARN_MB_FLAG','MB_SP_DOZING',1),
-- F_YARN_WASTE_LESS_MB_OPU: WASTE_PERC, MB_SP_DOZING, OPU
  ('F_YARN_WASTE_LESS_MB_OPU','WASTE_PERC',1),('F_YARN_WASTE_LESS_MB_OPU','MB_SP_DOZING',2),('F_YARN_WASTE_LESS_MB_OPU','OPU',3),
-- F_YARN_HEATSET_KG_COND: HEATSET_COST_PER_BATCH, BATCH_WEIGHT
  ('F_YARN_HEATSET_KG_COND','HEATSET_COST_PER_BATCH',1),('F_YARN_HEATSET_KG_COND','BATCH_WEIGHT',2),
-- FROM_MARKETING formulas have no formula_param rows — engine resolves from MKT session directly
-- SNAPSHOT: DELIVERY_COST_BEFORE_QLOSS
  ('F_YARN_DEL_QLOSS_PRE_PROC','DELIVERY_COST_BEFORE_QLOSS',1)
) AS fp(fcode, pcode, sort_order)
WHERE
    (SELECT id FROM mst_formula WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND (SELECT id FROM mst_parameter WHERE param_code = fp.pcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND NOT EXISTS (
    SELECT 1 FROM formula_param fp2
    WHERE fp2.formula_id = (SELECT id FROM mst_formula WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1)
      AND fp2.param_id   = (SELECT id FROM mst_parameter WHERE param_code = fp.pcode AND deleted_at IS NULL LIMIT 1)
);
