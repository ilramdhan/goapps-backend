-- Migration: V2 RM Cost engine — add per-detail valuation inputs to
-- cst_rm_group_detail. These feed CL/SL/FL formulas in cst_rm_cost_detail.

ALTER TABLE cst_rm_group_detail
    ADD COLUMN IF NOT EXISTS valuation_freight_rate     DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS valuation_anti_dumping_pct DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS valuation_duty_pct         DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS valuation_transport_rate   DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS valuation_default_value    DECIMAL(20,8);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_detail_valuation_freight_rate_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_detail
            ADD CONSTRAINT chk_rm_group_detail_valuation_freight_rate_nonneg
            CHECK (valuation_freight_rate IS NULL OR valuation_freight_rate >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_detail_valuation_anti_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_detail
            ADD CONSTRAINT chk_rm_group_detail_valuation_anti_nonneg
            CHECK (valuation_anti_dumping_pct IS NULL OR valuation_anti_dumping_pct >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_detail_valuation_duty_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_detail
            ADD CONSTRAINT chk_rm_group_detail_valuation_duty_nonneg
            CHECK (valuation_duty_pct IS NULL OR valuation_duty_pct >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_detail_valuation_transport_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_detail
            ADD CONSTRAINT chk_rm_group_detail_valuation_transport_nonneg
            CHECK (valuation_transport_rate IS NULL OR valuation_transport_rate >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_detail_valuation_default_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_detail
            ADD CONSTRAINT chk_rm_group_detail_valuation_default_nonneg
            CHECK (valuation_default_value IS NULL OR valuation_default_value >= 0);
    END IF;
END$$;

COMMENT ON COLUMN cst_rm_group_detail.valuation_freight_rate IS 'V2: Per-detail freight rate, feeds CL/SL/FL chain.';
COMMENT ON COLUMN cst_rm_group_detail.valuation_anti_dumping_pct IS 'V2: Decimal (0.10 = 10%). Computed but excluded from CL/SL sum; included in FL sum (per Excel reference).';
COMMENT ON COLUMN cst_rm_group_detail.valuation_duty_pct IS 'V2: Decimal. Used in CL/SL/FL.';
COMMENT ON COLUMN cst_rm_group_detail.valuation_transport_rate IS 'V2: Per-detail transport rate, feeds CL/SL/FL.';
COMMENT ON COLUMN cst_rm_group_detail.valuation_default_value IS 'V2: Drives FL when set; NULL means FL chain → 0.';
