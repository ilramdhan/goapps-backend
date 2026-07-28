-- Sales order staging: ETL inbox from Oracle MGT_SO_PENDING_WEB. Full-replace each
-- sync; sos_pulled_to_demand_id is preserved across replaces (NULL = still in LOV).
-- sos_contract_sys_id is a soft ref to finance (separate DB, no FK).
CREATE TABLE IF NOT EXISTS sales_order_staging (
    sos_id                  BIGSERIAL     PRIMARY KEY,
    sos_contract_no         VARCHAR(50),
    sos_contract_date       DATE,
    sos_contract_sys_id     BIGINT,
    sos_customer_code       VARCHAR(20),
    sos_customer_name       VARCHAR(100),
    sos_item_code           VARCHAR(30),
    sos_item_desc           VARCHAR(100),
    sos_grade_code          VARCHAR(20),
    sos_shade_code          VARCHAR(20),
    sos_shade_name          VARCHAR(100),
    sos_qty_ordered         DECIMAL(18,3),
    sos_qty_delivered       DECIMAL(18,3),
    sos_qty_remaining       DECIMAL(18,3),
    sos_deadline            DATE,
    sos_ship_date           VARCHAR(20),
    sos_merge_no            VARCHAR(20),
    sos_term                VARCHAR(20),
    sos_rate                DECIMAL(12,4),
    sos_currency            VARCHAR(5),
    sos_blocked_status      VARCHAR(50),
    sos_outstanding_ar      DECIMAL(18,2),
    sos_pallet_type         VARCHAR(20),
    sos_end_use             VARCHAR(50),
    sos_mix_flag            VARCHAR(1),
    sos_annotation          VARCHAR(200),
    sos_remarks             VARCHAR(200),
    sos_etl_synced_at       TIMESTAMPTZ,
    sos_pulled_to_demand_id BIGINT        -- NULL = available in LOV, set = already pulled
);

CREATE INDEX IF NOT EXISTS idx_sos_unpulled ON sales_order_staging (sos_id) WHERE sos_pulled_to_demand_id IS NULL;
