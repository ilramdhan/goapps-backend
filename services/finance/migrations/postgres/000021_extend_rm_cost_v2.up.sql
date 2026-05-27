-- Migration: V2 RM Cost engine — add snapshot + computed-rate columns to
-- cst_rm_cost. The new columns coexist with the existing v1 columns; the
-- calculation engine populates the new ones, the existing flag_* / cost_*
-- columns stay populated for back-compat reads.

ALTER TABLE cst_rm_cost
    ADD COLUMN IF NOT EXISTS valuation_flag_v2          VARCHAR(10),
    ADD COLUMN IF NOT EXISTS marketing_flag_v2          VARCHAR(10),
    ADD COLUMN IF NOT EXISTS marketing_freight_rate     DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS marketing_anti_dumping_pct DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS marketing_duty_pct         DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS marketing_transport_rate   DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS marketing_default_value    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS simulation_rate            DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS cl_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS sl_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS fl_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS sp_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS pp_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS fp_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS cr_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS sr_rate                    DECIMAL(20,8),
    ADD COLUMN IF NOT EXISTS pr_rate                    DECIMAL(20,8);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_cost_valuation_flag_v2'
    ) THEN
        ALTER TABLE cst_rm_cost
            ADD CONSTRAINT chk_rm_cost_valuation_flag_v2
            CHECK (valuation_flag_v2 IS NULL OR valuation_flag_v2 IN ('AUTO','CR','SR','PR','CL','SL','FL'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_rm_cost_marketing_flag_v2'
    ) THEN
        ALTER TABLE cst_rm_cost
            ADD CONSTRAINT chk_rm_cost_marketing_flag_v2
            CHECK (marketing_flag_v2 IS NULL OR marketing_flag_v2 IN ('AUTO','SP','PP','FP'));
    END IF;
END$$;

COMMENT ON COLUMN cst_rm_cost.valuation_flag_v2 IS 'V2: AUTO/CR/SR/PR/CL/SL/FL — drives selection of cost_val.';
COMMENT ON COLUMN cst_rm_cost.marketing_flag_v2 IS 'V2: AUTO/SP/PP/FP — drives selection of cost_mark.';
COMMENT ON COLUMN cst_rm_cost.simulation_rate IS 'V2: Editable per-row input. cost_sim is recomputed from this + marketing snapshot.';
COMMENT ON COLUMN cst_rm_cost.fl_rate IS 'V2: MAX(fix_landed_cost) across detail rows (per Excel reference).';
