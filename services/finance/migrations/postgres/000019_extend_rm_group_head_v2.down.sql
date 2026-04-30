ALTER TABLE cst_rm_group_head
    DROP CONSTRAINT IF EXISTS chk_rm_group_valuation_flag_v2,
    DROP CONSTRAINT IF EXISTS chk_rm_group_marketing_flag_v2,
    DROP CONSTRAINT IF EXISTS chk_rm_group_marketing_freight_rate_nonneg,
    DROP CONSTRAINT IF EXISTS chk_rm_group_marketing_anti_dumping_nonneg,
    DROP CONSTRAINT IF EXISTS chk_rm_group_marketing_default_value_nonneg,
    DROP COLUMN IF EXISTS marketing_freight_rate,
    DROP COLUMN IF EXISTS marketing_anti_dumping_pct,
    DROP COLUMN IF EXISTS marketing_default_value,
    DROP COLUMN IF EXISTS valuation_flag,
    DROP COLUMN IF EXISTS marketing_flag;
