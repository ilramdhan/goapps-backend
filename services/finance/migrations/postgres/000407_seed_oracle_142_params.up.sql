-- Extend varchar columns to accommodate Oracle param codes (max 41 chars).
ALTER TABLE mst_parameter ALTER COLUMN param_code TYPE VARCHAR(50);
ALTER TABLE mst_parameter ALTER COLUMN lookup_fill_group_code TYPE VARCHAR(50);

-- ============================================================
-- PART 1: Insert 142 params
-- Columns: param_code, param_name, param_short_name, data_type,
--          param_category, uom_id (via subquery), default_value,
--          min_value, max_value, is_active, created_by
-- ============================================================

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, min_value, max_value, is_active,
    created_at, created_by
)
SELECT
    p.code, p.name, p.short_name, p.data_type, p.category,
    u.uom_id, p.default_val::NUMERIC, p.min_val::NUMERIC, p.max_val::NUMERIC, TRUE,
    NOW(), 'seed_oracle_142'
FROM (VALUES
-- INPUT params
  ('A9_PERC','A9 (Percentage)','A9 (Percentage)','NUMBER','INPUT','PCT',NULL,'0','100'),
  ('A9_WT','A9 - Weight','A9 - Weight','NUMBER','INPUT','KG',NULL,NULL,NULL),
  ('ACT_DENIER','Actual Denier','Actual Denier','NUMBER','INPUT','DEN',NULL,'0','9999'),
  ('AE_PERC','AE (Percentage)','AE (Percentage)','NUMBER','INPUT','PCT',NULL,'0','100'),
  ('AE_WT','AE - Weight','AE - Weight','NUMBER','INPUT','KG',NULL,NULL,NULL),
  ('AX_PERC','AX (Percentage)','AX (Percentage)','NUMBER','INPUT','PCT',NULL,'0','100'),
  ('AX_WT','AX - Weight','AX - Weight','NUMBER','INPUT','KG',NULL,'0','100'),
  ('A_PERC','A (Percentage)','A (Percentage)','NUMBER','INPUT','PCT',NULL,'0','100'),
  ('A_WT','A - Weight','A - Weight','NUMBER','INPUT','KG',NULL,NULL,NULL),
  ('B_PERC','B (Percentage)','B (Percentage)','NUMBER','INPUT','PCT',NULL,'0','100'),
  ('B_WT','B - Weight','B - Weight','NUMBER','INPUT','KG',NULL,NULL,NULL),
  ('C_PERC','C (Percentage)','C (Percentage)','NUMBER','INPUT','PCT',NULL,'0','100'),
  ('C_WT','C - Weight','C - Weight','NUMBER','INPUT','KG',NULL,NULL,NULL),
  ('CAPTIVE_NO_OF_BOB','Captive No of Bobbins','Captive No of Bobbins','NUMBER','INPUT','PCS','6','1','100'),
  ('COSTING_LINK','Marketing Costing Link','','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('CROSS_SECTION','Cross Section','Cross Section','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('DENIER','Denier','Denier','NUMBER','INPUT','DEN',NULL,'0','9999'),
  ('DELIVERY_NO_OF_BOB','Delivery No of Bobbins','Delivery No of Bobbins','NUMBER','INPUT','PCS','6','1','100'),
  ('DOZING_ADJUST','Dozing Adjust','Dozing Adjust','NUMBER','INPUT',NULL,NULL,NULL,NULL),
  ('HEATSET_CODE','Heatset Code','Heatset Code','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('HEATSET_COST_PER_BATCH','Heatset Cost per Batch','Heatset Cost per Batch','NUMBER','INPUT','USD',NULL,NULL,NULL),
  ('INTERMINGLE','Intermingle','Intermingle','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('ITEM_NAME','Item Name','','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('MC_EFFICIENCY','Machine Efficiency','Machine Efficiency','NUMBER','INPUT',NULL,'95','50','100'),
  ('MC_NAME','Machine Name','Machine Name','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('MC_SPEED','Machine Speed','Machine Speed','NUMBER','INPUT',NULL,NULL,'0','9999'),
  ('NO_BOB_PER_TROLLIES','No. Of Bobbins Per Trolley','No. Of Bobbins Per Trolley','NUMBER','INPUT','PCS',NULL,'0','999'),
  ('NO_OF_END','No. Of End','No. Of End','NUMBER','INPUT',NULL,'1','1','100'),
  ('NO_OF_FILAMENTS','No. Of Filaments','No. Of Filaments','NUMBER','INPUT',NULL,NULL,'1','999'),
  ('NO_OF_PLY','No. Of Ply','No. Of Ply','NUMBER','INPUT',NULL,'1','1','12'),
  ('NO_OF_POSITION','No. Of Position','No. Of Position','NUMBER','INPUT',NULL,NULL,'1','9999'),
  ('NO_OF_TROLLIES','No. Of Trollies','No. Of Trollies','NUMBER','INPUT','PCS',NULL,'0','999'),
  ('OPU','OPU','OPU','NUMBER','INPUT','PCT','2.2','0','50'),
  ('ORION_ITEM','Item Code','','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('ORION_LINK','Orion Link','','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('PRODUCT_INDEX','Production Index','Production Index','NUMBER','INPUT',NULL,NULL,NULL,NULL),
  ('SHADE_CODE','Shade Code','','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('SHADE_NAME','Shade Name','','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('SOFTNER_COST','Softner Cost','Softner Cost','NUMBER','INPUT','USD','0',NULL,NULL),
  ('SPECIAL_COST_1','Special Cost 1','Special Cost 1','NUMBER','INPUT','USD',NULL,NULL,NULL),
  ('SPECIAL_COST_FLAG','Special Cost Flag','Special Cost Flag','TEXT','INPUT',NULL,NULL,NULL,NULL),
  ('TPM','TPM','TPM','NUMBER','INPUT',NULL,NULL,'0','9999'),
  ('WASTE_PERC','Waste Percentage','Waste Percentage','NUMBER','INPUT','PCT','0.7','0','20'),
  ('Y_TYPE','Y - Type','Y - Type','TEXT','INPUT',NULL,NULL,NULL,NULL),
-- RATE params
  ('OIL_RATE','Oil Rate','Oil Rate','NUMBER','RATE','USD',NULL,NULL,NULL),
  ('RM_LANDED_COST','Raw Material Landed cost','Raw Material Landed cost','NUMBER','RATE','USD',NULL,NULL,NULL),
  ('RM_RATE','Raw Material Rate','Raw Material Rate','NUMBER','RATE','USD',NULL,NULL,NULL),
-- CALCULATED params
  ('ADDITIONAL_VAL_LOSS','Additional Value Loss','Additional Value Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('ADD_NON_STD_BC_LOSS','Additional Non Standard BC Loss','Additional Non Standard BC Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('BATCH_WEIGHT','Batch Weight','Batch Weight','NUMBER','CALCULATED','KG',NULL,NULL,NULL),
  ('BC_SP','BC Special Product','BC Special Product','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('BC_VAL_LOSS_CAPTIVE','BC Value Loss (Captive)','BC Value Loss (Captive)','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('BC_VAL_LOSS_DELIVERY','BC Value Loss (Delivery)','BC Value Loss (Delivery)','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('CAPTIVE_BOX_WT','Captive Box Weight','Captive Box Weight','NUMBER','CALCULATED','KG',NULL,NULL,NULL),
  ('CAPTIVE_CONVERSION','Captive Conversion Cost (VAL)','Cap Conversion','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('CAPTIVE_COST_BEFORE_QLOSS','Captive Cost Before Quality Loss','Captive Cost Before Quality Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('CAPTIVE_COST_QLTY_LOSS','Captive Cost with Quality Loss','Captive Cost with Quality Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('CAPTIVE_PACK_COST','Captive Packing Cost','Captive Packing Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('CONV_FACTOR','Conversion Factor','Conversion Factor','NUMBER','CALCULATED',NULL,NULL,NULL,NULL),
  ('DELIVERY_BOX_WT','Delivery Box Weight','Delivery Box Weight','NUMBER','CALCULATED','KG',NULL,NULL,NULL),
  ('DELIVERY_CONVERSION','Delivery Conversion Cost (VAL)','Del Conversion','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('DELIVERY_COST_BEFORE_QLOSS','Delivery Cost Before Quality Loss','Delivery Cost Before Quality Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('DELIVERY_COST_BEFORE_QLOSS_ADD_PER','% Add Top 95','% Add Top 95','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('DELIVERY_COST_BEFORE_QLOSS_ADDITION','Top 95 X % Add','Top 95 X % Add','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('DELIVERY_COST_BEFORE_QLOSS_BEFORE_PROCESS','Value Top 95 before Process','Value Top 95 before Process','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('DELIVERY_COST_QLTY_LOSS','Delivery Cost with Quality Loss','Delivery Cost with Quality Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('DELIVERY_PACK_COST','Delivery Packing Cost','Delivery Packing Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('HEATSET_COST_PER_KG','Heatset Cost per Kg','Heatset Cost per Kg','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('INTERMINGLING','Intermingling','Intermingling','NUMBER','CALCULATED',NULL,NULL,NULL,NULL),
  ('MANPOWER_PER_KG','Manpower Per Kgs','Manpower Per Kgs','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('MB_COST_MKT','MB Cost Marketing','MB Cost Marketing','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('MB_FLAG','Masterbatch Flag','Masterbatch Flag','TEXT','CALCULATED',NULL,NULL,NULL,NULL),
  ('NET_BOB_WT','Net Bobbin Weight','Net Bobbin Weight','NUMBER','CALCULATED','KG',NULL,NULL,NULL),
  ('NET_PRODUCTION','Net Production','Net Production','NUMBER','CALCULATED','KG',NULL,NULL,NULL),
  ('NON_STD_BC_SP','Non Standard BC SP','Non Standard BC SP','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('NON_STD_VALUE_LOSS','Non Standard Value Loss','Non Standard Value Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('OIL_COST','Oil Cost','Oil Cost','NUMBER','CALCULATED','KG',NULL,NULL,NULL),
  ('OIL_GAIN','Oil Gain','Oil Gain','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('OIL_NAME','Oil Name','Oil Name','TEXT','CALCULATED',NULL,NULL,NULL,NULL),
  ('ONLY_CONV_CAP_PACK_EXCL_MB','Only Conversion Captive Packing ex MB','Only Conversion Captive Packing ex MB','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('ONLY_CONV_DEL_PACK_EXCL_MB','Only Conversion Delivery Packing ex MB','Only Conversion Delivery Packing ex MB','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('OVERHEAD_PER_KG','Overhead Per Kgs','Overhead Per Kgs','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('POWER_PER_KG','Power Per Kgs','Power Per Kgs','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('QLTY_LOSS_CAPTIVE_COST','Quality Loss Captive Cost','Quality Loss Captive Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('QLTY_LOSS_DELIVERY_COST','Quality Loss Delivery Cost','Quality Loss Delivery Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_AE_A9_A','R AE / A9 / A','R AE / A9 / A','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_AX','R AX','R AX','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_BC','R BC','R BC','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_BC_LOSS','R BC Loss','R BC Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_NON_STD_DIFF','R Non Standard Difference','R Non Standard Difference','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_NON_STD_LOSS','R Non Standard Loss','R Non Standard Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('R_NON_STD_SP','R Non Standard SP','R Non Standard SP','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('RAW_MATERIAL','Raw Material','Raw Material','TEXT','CALCULATED',NULL,NULL,NULL,NULL),
  ('RM_NORMS','Raw Material Norms','Raw Material Norms','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('RM_TYPE','RM Table Type','RM Table Type','TEXT','CALCULATED',NULL,NULL,NULL,NULL),
  ('RP_CC','RP-CC','RP-CC','TEXT','CALCULATED',NULL,NULL,NULL,NULL),
  ('RP_DOZING','RP-Dozing','RP-Dozing','NUMBER','CALCULATED',NULL,NULL,NULL,NULL),
  ('SPARESCOST_PER_KG','Consumables and Spares Per Kgs','Consumables and Spares Per Kgs','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('SPECIAL_COST_2','Special Cost 2','Special Cost 2','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('STEAM_COST_CNG','Steam Cost (CNG)','Steam Cost (CNG)','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('TOTAL_FIXEDCOST_PER_KG','Total','Total','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_1_DEL_COST','Volume Bucket 1 - Delivery Cost','Volume Bucket 1 - Delivery Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_1_LOSS','Volume Bucket 1 - Loss','Volume Bucket 1 - Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_2_DEL_COST','Volume Bucket 2 - Delivery Cost','Volume Bucket 2 - Delivery Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_2_LOSS','Volume Bucket 2 - Loss','Volume Bucket 2 - Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_3_DEL_COST','Volume Bucket 3 - Delivery Cost','Volume Bucket 3 - Delivery Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_3_LOSS','Volume Bucket 3 - Loss','Volume Bucket 3 - Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_4_DEL_COST','Volume Bucket 4 - Delivery Cost','Volume Bucket 4 - Delivery Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_4_LOSS','Volume Bucket 4 - Loss','Volume Bucket 4 - Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_5_DEL_COST','Volume Bucket 5 - Delivery Cost','Volume Bucket 5 - Delivery Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('VOLUME_BUCKET_5_LOSS','Volume Bucket 5 - Loss','Volume Bucket 5 - Loss','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('WASHING_COST','Washing Cost','Washing Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
  ('WASTE_LESS_MB_OPU','Waste Less MB dozing, OPU','Waste Less MB dozing, OPU','NUMBER','CALCULATED','USD',NULL,NULL,NULL),
-- MASTER_LOOKUP params
  ('BC_SPECIAL_PROD','BC Special Product','BC Special Product','NUMBER','MASTER_LOOKUP','USD',NULL,'0','100'),
  ('CAPTIVE_BOB_RATE','Captive Bobbin Rate','Captive Bobbin Rate','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('CAPTIVE_BOX_RATE','Captive Box Rate','Captive Box Rate','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('CAPTIVE_PACK_CODE','Captive Pack Code','Captive Pack Code','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('CHANGE_OVER_QLTY_LOSS','Change Over Quantity Loss (Info)','Change Over Quantity Loss (Info)','NUMBER','MASTER_LOOKUP','USD','100','0','9999'),
  ('CUSTOMER','Customer','Customer','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('DELIVERY_BOB_RATE','Delivery Bobbin Rate','Delivery Bobbin Rate','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('DELIVERY_BOX_RATE','Delivery Box Rate','Delivery Box Rate','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('DELIVERY_PACK_CODE','Delivery Pack Code','Delivery Pack Code','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('MANPOWER_PER_DAY','Manpower Per Day','Manpower Per Day','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('MB_RATE_MKT','MB Rate Marketing','MB Rate Marketing','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('MB_SP_CC','MB / SP - CC','MB / SP - CC','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('MB_SP_CODE','MB / SP Code','MB / SP Code','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('MB_SP_DENIER','MB / SP - Denier','MB / SP - Denier','NUMBER','MASTER_LOOKUP','DEN',NULL,NULL,NULL),
  ('MB_SP_DOZING','MB / SP - Dozing','MB / SP - Dozing','NUMBER','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('MB_SP_DYE','MB / SP Dye Name','MB / SP Dye Name','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('MB_SP_FILAMENT','MB / SP - Filament','MB / SP - Filament','NUMBER','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('NON_STD_SPECIAL_PROD','Non Standard Special Product','Non Standard Special Product','NUMBER','MASTER_LOOKUP','USD',NULL,'0','100'),
  ('OVERHEAD_PER_HEAD','Overhead Per Day','Overhead Per Day','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('POWER_PER_DAY','Power Per Day','Power Per Day','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('SOFTNER_COST_LOOKUP','Softner Cost (placeholder)','Softner Cost','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('SPARESCOST_PER_DAY','Spares Cost / Day','Spares Cost / Day','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL),
  ('STD_VALUE_LOSS','Standard Value Loss','Standard Value Loss','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('VALUATION','Valuation','Valuation','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL),
  ('VALUE_LOSS','Value loss','Value loss','TEXT','MASTER_LOOKUP',NULL,NULL,'0','100'),
  ('VOLUME_BUCKET_1_QTY','Volume Bucket 1 - Quantity','Volume Bucket 1 - Quantity','NUMBER','MASTER_LOOKUP','USD',NULL,'0','99999'),
  ('VOLUME_BUCKET_2_QTY','Volume Bucket 2 - Quantity','Volume Bucket 2 - Quantity','NUMBER','MASTER_LOOKUP','USD',NULL,'0','99999'),
  ('VOLUME_BUCKET_3_QTY','Volume Bucket 3 - Quantity','Volume Bucket 3 - Quantity','NUMBER','MASTER_LOOKUP','USD',NULL,'0','99999'),
  ('VOLUME_BUCKET_4_QTY','Volume Bucket 4 - Quantity','Volume Bucket 4 - Quantity','NUMBER','MASTER_LOOKUP','USD',NULL,'0','99999'),
  ('VOLUME_BUCKET_5_QTY','Volume Bucket 5 - Quantity','Volume Bucket 5 - Quantity','NUMBER','MASTER_LOOKUP','USD',NULL,'0','99999')
) AS p(code, name, short_name, data_type, category, uom_code, default_val, min_val, max_val)
LEFT JOIN mst_uom u ON u.uom_code = p.uom_code AND u.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Set trigger params' lookup_master_code
-- ============================================================

UPDATE mst_parameter SET
    lookup_master_code = 'MACHINE',
    updated_at = NOW(), updated_by = 'seed_oracle_142'
WHERE param_code = 'MC_NAME' AND deleted_at IS NULL;

UPDATE mst_parameter SET
    lookup_master_code = 'INTERMINGLING',
    updated_at = NOW(), updated_by = 'seed_oracle_142'
WHERE param_code = 'INTERMINGLE' AND deleted_at IS NULL;

UPDATE mst_parameter SET
    lookup_master_code = 'BOX_BOBBIN_COST',
    updated_at = NOW(), updated_by = 'seed_oracle_142'
WHERE param_code IN ('CAPTIVE_PACK_CODE', 'DELIVERY_PACK_CODE') AND deleted_at IS NULL;

UPDATE mst_parameter SET
    lookup_master_code = 'MB_SPIN',
    updated_at = NOW(), updated_by = 'seed_oracle_142'
WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL;

-- ============================================================
-- PART 3: Set fill-group children (lookup_fill_group_code + lookup_source_column)
-- ============================================================

-- MC_NAME children
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='mc_speed',        updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MC_SPEED'           AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='mc_efficiency',   updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MC_EFFICIENCY'       AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='no_of_position',  updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='NO_OF_POSITION'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='no_of_end',       updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='NO_OF_END'           AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='power_per_day',   updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='POWER_PER_DAY'       AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='mp_per_day',      updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MANPOWER_PER_DAY'    AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='ohs_per_day',     updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='OVERHEAD_PER_HEAD'   AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='spares_per_day',  updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='SPARESCOST_PER_DAY'  AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='kgs_lost_change', updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='CHANGE_OVER_QLTY_LOSS' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='vb1_qty',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='VOLUME_BUCKET_1_QTY' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='vb2_qty',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='VOLUME_BUCKET_2_QTY' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='vb3_qty',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='VOLUME_BUCKET_3_QTY' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='vb4_qty',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='VOLUME_BUCKET_4_QTY' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME', lookup_source_column='vb5_qty',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='VOLUME_BUCKET_5_QTY' AND deleted_at IS NULL;

-- INTERMINGLE child
UPDATE mst_parameter SET lookup_fill_group_code='INTERMINGLE', lookup_source_column='intm_cost_per_kg', updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='INTERMINGLING' AND deleted_at IS NULL;

-- CAPTIVE_PACK_CODE children
UPDATE mst_parameter SET lookup_fill_group_code='CAPTIVE_PACK_CODE', lookup_source_column='bbcr_bob_rate_mkt', updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='CAPTIVE_BOB_RATE' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='CAPTIVE_PACK_CODE', lookup_source_column='bbcr_box_rate_mkt', updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='CAPTIVE_BOX_RATE' AND deleted_at IS NULL;

-- DELIVERY_PACK_CODE children
UPDATE mst_parameter SET lookup_fill_group_code='DELIVERY_PACK_CODE', lookup_source_column='bbcr_bob_rate_val', updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='DELIVERY_BOB_RATE' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='DELIVERY_PACK_CODE', lookup_source_column='bbcr_box_rate_val', updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='DELIVERY_BOX_RATE' AND deleted_at IS NULL;

-- MB_SP_CODE children
UPDATE mst_parameter SET lookup_fill_group_code='MB_SP_CODE', lookup_source_column='mbs_denier',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MB_SP_DENIER'   AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MB_SP_CODE', lookup_source_column='mbs_dozing',         updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MB_SP_DOZING'   AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MB_SP_CODE', lookup_source_column='mbs_mgt_name',       updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MB_SP_DYE'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MB_SP_CODE', lookup_source_column='mbs_filament',       updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MB_SP_FILAMENT' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MB_SP_CODE', lookup_source_column='mbs_cc',             updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MB_SP_CC'       AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='MB_SP_CODE', lookup_source_column='mbs_cost_rate_mkt',  updated_at=NOW(), updated_by='seed_oracle_142' WHERE param_code='MB_RATE_MKT'    AND deleted_at IS NULL;
