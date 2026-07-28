-- 000030: record the CPM product resolution outcome on each staging row.
--
-- ppc_db and finance_db are separate databases, so the (erp_item_code,
-- shade_code) -> cost_product_master lookup happens over gRPC
-- (finance.v1.CostMasterLookupService.ResolveCostProductMasterByErpCode) and
-- the result is cached here. Without this, demand pull had no product source
-- and fell back to writing a contract id into a product column.

ALTER TABLE sales_order_staging
  ADD COLUMN IF NOT EXISTS sos_cpm_product_sys_id BIGINT,
  ADD COLUMN IF NOT EXISTS sos_match_status VARCHAR(20) NOT NULL DEFAULT 'UNRESOLVED',
  ADD COLUMN IF NOT EXISTS sos_match_count INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS sos_matched_at TIMESTAMPTZ;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_sos_match_status'
  ) THEN
    ALTER TABLE sales_order_staging
      ADD CONSTRAINT chk_sos_match_status
      CHECK (sos_match_status IN ('UNRESOLVED', 'AUTO', 'AMBIGUOUS', 'NOT_FOUND', 'MANUAL'));
  END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_sos_match_status
  ON sales_order_staging (sos_match_status)
  WHERE sos_pulled_to_demand_id IS NULL;

COMMENT ON COLUMN sales_order_staging.sos_cpm_product_sys_id IS
  'Resolved cost_product_master.cpm_product_sys_id (finance_db, resolved over gRPC). NULL until resolved.';
COMMENT ON COLUMN sales_order_staging.sos_match_status IS
  'UNRESOLVED = not attempted; AUTO = unique automatic match; AMBIGUOUS = multiple CPM rows; NOT_FOUND = no CPM row; MANUAL = picked by a planner.';
COMMENT ON COLUMN sales_order_staging.sos_match_count IS
  'Number of CPM rows matching (erp_item_code, shade_code) at resolution time.';
