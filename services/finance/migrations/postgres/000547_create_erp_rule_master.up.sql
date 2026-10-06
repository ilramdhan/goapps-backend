-- 000547 — ERP rule master (design Part 1 §4.1; plan-01 P0-T2).
--
-- GoApps owns the valuation rules for non-AX ERP grades (legacy MGTDAT
-- MGT_ITEM_COST_VAL_LOSS / IM_VS_STATIC_VALUE 'ITEMSELLPRIC' /
-- OM_GRADE_CODE_1.GRADE_BL_SHORT_NAME). Finance maintains them in GoApps;
-- there is no rule ETL from Oracle. This file is schema only; the seed is
-- 000548 (schema/data split, design X-1).
--
-- Objects:
--   * cost_erp_grade.ceg_grade_group (+ idx_ceg_grade_group): nullable, owned
--     by GoApps. The replica sync of cost_erp_grade (P0-T15) NEVER writes it.
--   * cst_erp_grade_group_seed: the 22 grade->group mappings. Prod
--     cost_erp_grade is empty (sql-1 F-2), so the mapping is kept here and
--     re-applied after the first master sync (P0-T15b), filling NULLs only.
--   * cst_erp_sell_price: reference selling price per basis (USD/kg).
--   * cst_erp_valloss_rule: basis + value loss per (fg_type, prod_type,
--     grade_group). Basis-to-price is checked by the service / V-08, not by
--     an FK ('COST' is not a price row).
--
-- Additive only: no existing column, row or output changes. Down drops only
-- what this file created.

BEGIN;

ALTER TABLE cost_erp_grade
    ADD COLUMN IF NOT EXISTS ceg_grade_group VARCHAR(20) NULL;

COMMENT ON COLUMN cost_erp_grade.ceg_grade_group IS
    'ERP grade group (NS/AE/BC/BB/JLT/POYA/AX). Owned by GoApps (000547); the Oracle replica sync never writes this column.';

CREATE INDEX IF NOT EXISTS idx_ceg_grade_group
    ON cost_erp_grade (ceg_grade_group)
    WHERE ceg_grade_group IS NOT NULL;

CREATE TABLE IF NOT EXISTS cst_erp_grade_group_seed (
    cggs_grade_code  VARCHAR(20) PRIMARY KEY,
    cggs_grade_group VARCHAR(20) NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by       VARCHAR(64) NOT NULL DEFAULT 'MIGRATION'
);

COMMENT ON TABLE cst_erp_grade_group_seed IS
    'ERP grade -> grade group mapping (000547/000548). Applied to cost_erp_grade (NULLs only) by 000548 and after each master sync (P0-T15b).';

CREATE TABLE IF NOT EXISTS cst_erp_sell_price (
    cesp_basis     VARCHAR(15)   PRIMARY KEY,
    cesp_price     NUMERIC(20,6) NOT NULL,
    cesp_is_active BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by     VARCHAR(64)   NOT NULL DEFAULT 'MIGRATION',
    updated_at     TIMESTAMPTZ,
    updated_by     VARCHAR(64),
    CONSTRAINT chk_cesp_basis CHECK (cesp_basis IN ('SPPTY', 'SPITY', 'SPBSD')),
    CONSTRAINT chk_cesp_price CHECK (cesp_price > 0)
);

COMMENT ON TABLE cst_erp_sell_price IS
    'Reference selling price per ERP valuation basis, USD/kg (legacy IM_VS_STATIC_VALUE ITEMSELLPRIC). Finance-owned.';

CREATE TABLE IF NOT EXISTS cst_erp_valloss_rule (
    cevr_id          SERIAL        PRIMARY KEY,
    cevr_fg_type     VARCHAR(15)   NOT NULL,
    cevr_prod_type   VARCHAR(3)    NOT NULL,
    cevr_grade_group VARCHAR(20)   NOT NULL,
    cevr_basis       VARCHAR(15)   NOT NULL,
    cevr_val_loss    NUMERIC(20,6) NOT NULL DEFAULT 0,
    cevr_is_active   BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by       VARCHAR(64)   NOT NULL DEFAULT 'MIGRATION',
    updated_at       TIMESTAMPTZ,
    updated_by       VARCHAR(64),
    CONSTRAINT chk_cevr_prod_type   CHECK (cevr_prod_type IN ('POY', 'PTY', 'ITY')),
    CONSTRAINT chk_cevr_grade_group CHECK (cevr_grade_group <> 'AX'),
    CONSTRAINT chk_cevr_basis       CHECK (cevr_basis IN ('COST', 'SPPTY', 'SPITY', 'SPBSD')),
    CONSTRAINT chk_cevr_val_loss    CHECK (cevr_val_loss >= 0)
);

COMMENT ON TABLE cst_erp_valloss_rule IS
    'ERP value-loss rule per (FG type, prod type, grade group) (legacy MGT_ITEM_COST_VAL_LOSS). AX never goes through a rule. Finance-owned.';

-- One active rule per key; soft-deleted (inactive) history rows may repeat.
CREATE UNIQUE INDEX IF NOT EXISTS uk_cevr_key
    ON cst_erp_valloss_rule (cevr_fg_type, cevr_prod_type, cevr_grade_group)
    WHERE cevr_is_active;

CREATE INDEX IF NOT EXISTS idx_cevr_basis
    ON cst_erp_valloss_rule (cevr_basis);

COMMIT;
