ALTER TABLE cst_rm_group_detail
    DROP CONSTRAINT IF EXISTS chk_rm_group_detail_valuation_freight_rate_nonneg,
    DROP CONSTRAINT IF EXISTS chk_rm_group_detail_valuation_anti_nonneg,
    DROP CONSTRAINT IF EXISTS chk_rm_group_detail_valuation_duty_nonneg,
    DROP CONSTRAINT IF EXISTS chk_rm_group_detail_valuation_transport_nonneg,
    DROP CONSTRAINT IF EXISTS chk_rm_group_detail_valuation_default_nonneg,
    DROP COLUMN IF EXISTS valuation_freight_rate,
    DROP COLUMN IF EXISTS valuation_anti_dumping_pct,
    DROP COLUMN IF EXISTS valuation_duty_pct,
    DROP COLUMN IF EXISTS valuation_transport_rate,
    DROP COLUMN IF EXISTS valuation_default_value;
