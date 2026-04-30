-- Migration: V2 RM Cost engine — add marketing-only fields and new flag columns
-- to cst_rm_group_head. Existing cost_percentage / cost_per_kg are kept and
-- semantically aliased as Marketing Duty % / Marketing Transport Rate.

ALTER TABLE cst_rm_group_head
    ADD COLUMN IF NOT EXISTS marketing_freight_rate     DECIMAL(20,6),
    ADD COLUMN IF NOT EXISTS marketing_anti_dumping_pct DECIMAL(20,6),
    ADD COLUMN IF NOT EXISTS marketing_default_value    DECIMAL(20,6),
    ADD COLUMN IF NOT EXISTS valuation_flag             VARCHAR(10),
    ADD COLUMN IF NOT EXISTS marketing_flag             VARCHAR(10);

-- Domain constraints for the new flag columns. NULL allowed and treated as AUTO.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_valuation_flag_v2'
    ) THEN
        ALTER TABLE cst_rm_group_head
            ADD CONSTRAINT chk_rm_group_valuation_flag_v2
            CHECK (valuation_flag IS NULL OR valuation_flag IN ('AUTO','CR','SR','PR','CL','SL','FL'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_marketing_flag_v2'
    ) THEN
        ALTER TABLE cst_rm_group_head
            ADD CONSTRAINT chk_rm_group_marketing_flag_v2
            CHECK (marketing_flag IS NULL OR marketing_flag IN ('AUTO','SP','PP','FP'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_marketing_freight_rate_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_head
            ADD CONSTRAINT chk_rm_group_marketing_freight_rate_nonneg
            CHECK (marketing_freight_rate IS NULL OR marketing_freight_rate >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_marketing_anti_dumping_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_head
            ADD CONSTRAINT chk_rm_group_marketing_anti_dumping_nonneg
            CHECK (marketing_anti_dumping_pct IS NULL OR marketing_anti_dumping_pct >= 0);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_group_marketing_default_value_nonneg'
    ) THEN
        ALTER TABLE cst_rm_group_head
            ADD CONSTRAINT chk_rm_group_marketing_default_value_nonneg
            CHECK (marketing_default_value IS NULL OR marketing_default_value >= 0);
    END IF;
END$$;

COMMENT ON COLUMN cst_rm_group_head.marketing_freight_rate IS 'V2: Added to base rate before duty/anti in marketing projections (SP/PP/FP).';
COMMENT ON COLUMN cst_rm_group_head.marketing_anti_dumping_pct IS 'V2: Whole percent (e.g., 5 means 5%).';
COMMENT ON COLUMN cst_rm_group_head.marketing_default_value IS 'V2: Drives FP projection when set.';
COMMENT ON COLUMN cst_rm_group_head.valuation_flag IS 'V2: AUTO/CR/SR/PR/CL/SL/FL. NULL or AUTO means cascade fallback CL→SL→FL.';
COMMENT ON COLUMN cst_rm_group_head.marketing_flag IS 'V2: AUTO/SP/PP/FP. NULL or AUTO means cascade fallback SP→PP→FP.';
