-- R8 — activate the cost_erp_shade shell (000105) into a real master, ETL-synced
-- from Oracle MGTDAT.OM_GRADE_CODE_2 (2320 rows in production; column names
-- verified against ALL_TAB_COLUMNS, not guessed).
--
-- DECISION (see finance CLAUDE.md investigation): cost_erp_grade (000104) is NOT
-- touched by this migration. Its comment claims "(AX/AM/B/C)" quality grades, which
-- cannot be OM_GRADE_CODE_2 (2320 rows) — a 4-value quality grade table would never
-- have that cardinality. cost_erp_shade's comment ("NL/Z114S/Z108S etc") matches a
-- dye/color shade code shape and is already exposed read-only via
-- ListCostErpShades/CostErpShade in the finance proto, so this migration extends
-- that existing empty shell rather than creating a new parallel table.
--
-- String columns are widened to TEXT for the same reason migration 000036 (ppc
-- customer) uses TEXT: Oracle GRADE_CODE is VARCHAR2(40) and GRADE_NAME is
-- VARCHAR2(240) — both already exceed the shell's original VARCHAR(20)/VARCHAR(100),
-- so a sync insert could overflow before this migration (SQLSTATE 22001 risk).
ALTER TABLE cost_erp_shade
    ALTER COLUMN ces_shade_code TYPE TEXT,
    ALTER COLUMN ces_shade_name TYPE TEXT;

ALTER TABLE cost_erp_shade
    ADD COLUMN IF NOT EXISTS ces_shade_short_name  TEXT,          -- OM_GRADE_CODE_2.GRADE_SHORT_NAME
    ADD COLUMN IF NOT EXISTS ces_shade_source       VARCHAR(10)  NOT NULL DEFAULT 'ORACLE', -- ORACLE / MANUAL
    ADD COLUMN IF NOT EXISTS ces_source_created_at  TIMESTAMPTZ,  -- OM_GRADE_CODE_2.GRADE_CR_DT
    ADD COLUMN IF NOT EXISTS ces_source_updated_at  TIMESTAMPTZ,  -- OM_GRADE_CODE_2.GRADE_UPD_DT
    ADD COLUMN IF NOT EXISTS ces_source_created_by  VARCHAR(12),  -- OM_GRADE_CODE_2.GRADE_CR_UID (Oracle user id)
    ADD COLUMN IF NOT EXISTS ces_source_updated_by  VARCHAR(12),  -- OM_GRADE_CODE_2.GRADE_UPD_UID (Oracle user id)
    ADD COLUMN IF NOT EXISTS created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS created_by             VARCHAR(100) NOT NULL DEFAULT 'system',
    ADD COLUMN IF NOT EXISTS updated_at             TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_by             VARCHAR(100);

-- NOTE: GRADE_FRZ_FLAG_NUM -> ces_is_active mapping keeps the existing column
-- (ces_is_active BOOLEAN, added by 000105) unchanged. Application code maps it as
-- ces_is_active = NOT (GRADE_FRZ_FLAG_NUM = 1), mirroring the ppc customer_is_active
-- <- CUST_FRZ_FLAG_NUM convention. Unverified against production distribution — see
-- the aggregate SQL handed to the user for confirmation before the sync job ships.
--
-- NOTE: GRADE_BL_NAME / GRADE_BL_SHORT_NAME are deliberately NOT mirrored anywhere
-- in this migration or the Go code that follows. Their meaning ("BL") is unknown
-- and PRD/task guidance forbids inventing it. This is a decision gate — see report.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_cost_erp_shade_source'
    ) THEN
        ALTER TABLE cost_erp_shade
            ADD CONSTRAINT chk_cost_erp_shade_source CHECK (ces_shade_source IN ('ORACLE', 'MANUAL'));
    END IF;
END $$;

-- Case-insensitive code/name search, mirroring idx_customer_code_lower.
CREATE INDEX IF NOT EXISTS idx_cost_erp_shade_code_lower ON cost_erp_shade (LOWER(ces_shade_code));
CREATE INDEX IF NOT EXISTS idx_cost_erp_shade_name_lower ON cost_erp_shade (LOWER(ces_shade_name));

COMMENT ON TABLE cost_erp_shade IS 'PRD Phase B §7.3.3 — Shade master, ETL-synced read-mostly from Oracle MGTDAT.OM_GRADE_CODE_2 (natural key ces_shade_code = GRADE_CODE). Rows may also be hand-created (ces_shade_source = MANUAL) and are then never overwritten by sync. Also used by Phase A cost_product_spec for shade autocomplete.';
COMMENT ON COLUMN cost_erp_shade.ces_shade_short_name IS 'OM_GRADE_CODE_2.GRADE_SHORT_NAME.';
COMMENT ON COLUMN cost_erp_shade.ces_shade_source IS 'ORACLE = created/refreshed by sync; MANUAL = hand-created in finance and never overwritten by sync.';
COMMENT ON COLUMN cost_erp_shade.ces_is_active IS 'Derived from OM_GRADE_CODE_2.GRADE_FRZ_FLAG_NUM (1 = frozen -> inactive, same convention as ppc customer_is_active <- CUST_FRZ_FLAG_NUM). ~~Confirm distribution before relying on this in production.~~ DIPERBARUI 2026-08-26: measured on 2320 prod rows -- flag=1 on 17 rows (supports 1=inactive), NULL on 2301 rows (treated active), flag=2 on 2 rows (meaning UNKNOWN, currently defaults to active -- open decision gate, see isActiveFromFrzFlag in internal/infrastructure/oracle/shade_repository.go).';
COMMENT ON COLUMN cost_erp_shade.ces_source_created_by IS 'OM_GRADE_CODE_2.GRADE_CR_UID — Oracle-side user id, distinct from created_by (finance-side actor).';
COMMENT ON COLUMN cost_erp_shade.ces_source_updated_by IS 'OM_GRADE_CODE_2.GRADE_UPD_UID — Oracle-side user id, distinct from updated_by (finance-side actor).';
