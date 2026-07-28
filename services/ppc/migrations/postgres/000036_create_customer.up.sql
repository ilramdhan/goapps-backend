-- Customer master. SYNC-SOURCED from Oracle MGTDAT.OM_CUSTOMER (read-only SELECT),
-- mirroring the machine-master anti-drift pattern: rows are upserted by natural key
-- (customer_code) and PPC-local edits are preserved where they cannot be re-derived.
-- Only the columns PPC actually needs are mirrored -- OM_CUSTOMER carries ~60 more
-- (addresses, banks, credit terms, remarks) that belong to Orion, not to planning.
--
-- String columns are TEXT on purpose: migrations 000028/000029 exist only because
-- Oracle values overflowed VARCHAR on the SO-staging ETL (SQLSTATE 22001).
CREATE TABLE IF NOT EXISTS customer (
    customer_id          BIGSERIAL    PRIMARY KEY,
    customer_code        TEXT         NOT NULL,   -- OM_CUSTOMER.CUST_CODE (natural key)
    customer_name        TEXT         NOT NULL,   -- OM_CUSTOMER.CUST_NAME
    customer_short_name  TEXT,                    -- OM_CUSTOMER.CUST_SHORT_NAME
    customer_tax_no      TEXT,                    -- OM_CUSTOMER.CUST_TAX_REGN_NO (NPWP)
    customer_parent_code TEXT,                    -- OM_CUSTOMER.CUST_PARENT_CODE (group head)
    customer_is_active   BOOLEAN      NOT NULL DEFAULT TRUE,  -- NOT CUST_FRZ_FLAG_NUM
    customer_source      VARCHAR(10)  NOT NULL DEFAULT 'ORACLE', -- ORACLE / MANUAL
    source_created_at    TIMESTAMPTZ,             -- OM_CUSTOMER.CUST_CR_DT
    source_updated_at    TIMESTAMPTZ,             -- OM_CUSTOMER.CUST_UPD_DT
    synced_at            TIMESTAMPTZ,             -- last successful Oracle sync
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by           VARCHAR(100) NOT NULL DEFAULT 'system',
    updated_at           TIMESTAMPTZ,
    updated_by           VARCHAR(100),
    CONSTRAINT uq_customer_code UNIQUE (customer_code),
    CONSTRAINT chk_customer_source CHECK (customer_source IN ('ORACLE', 'MANUAL'))
);

-- Case-insensitive code/name search from the LOV combobox and the list page.
CREATE INDEX IF NOT EXISTS idx_customer_code_lower ON customer (LOWER(customer_code));
CREATE INDEX IF NOT EXISTS idx_customer_name_lower ON customer (LOWER(customer_name));
CREATE INDEX IF NOT EXISTS idx_customer_is_active ON customer (customer_is_active);

COMMENT ON TABLE customer IS 'PPC customer master, ETL-synced read-only from Oracle MGTDAT.OM_CUSTOMER.';
COMMENT ON COLUMN customer.customer_source IS 'ORACLE = created by sync; MANUAL = hand-created in PPC and never overwritten by sync.';
COMMENT ON COLUMN customer.customer_is_active IS 'Derived from OM_CUSTOMER.CUST_FRZ_FLAG_NUM (1 = frozen -> inactive).';
