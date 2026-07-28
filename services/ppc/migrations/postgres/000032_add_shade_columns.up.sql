-- 000032: carry shade (colour) through demand and plan item.
--
-- Shade is the customer-facing colour code of a contract line (e.g. 5918-01).
-- It already exists on the Orion staging row but was dropped when a demand was
-- pulled, so nothing downstream could see it. Plan items need it too: at the
-- finished-goods level every contract has its own shade, while at the upstream
-- (tty/pty/poy) levels the route product is usually natural — which is exactly
-- what makes those upstream items mergeable into a single work order.
--
-- idx_ppi_merge_key backs that merge-candidate lookup: same product, same
-- machine group, compatible shade, deadline within a window.

ALTER TABLE production_demand
  ADD COLUMN IF NOT EXISTS pd_shade_code VARCHAR(50),
  ADD COLUMN IF NOT EXISTS pd_shade_name VARCHAR(100);

ALTER TABLE production_plan_item
  ADD COLUMN IF NOT EXISTS ppi_shade_code VARCHAR(50),
  ADD COLUMN IF NOT EXISTS ppi_shade_name VARCHAR(100);

COMMENT ON COLUMN production_demand.pd_shade_code IS
  'Shade (colour) code copied from the Orion staging row at pull time. NULL when the contract line carries no shade.';
COMMENT ON COLUMN production_demand.pd_shade_name IS
  'Human-readable shade name copied from the Orion staging row at pull time.';
COMMENT ON COLUMN production_plan_item.ppi_shade_code IS
  'Shade (colour) code of the route product at this level. Natural (NL) at upstream levels, which is what makes those items mergeable.';
COMMENT ON COLUMN production_plan_item.ppi_shade_name IS
  'Human-readable shade name of the route product at this level.';

CREATE INDEX IF NOT EXISTS idx_ppi_merge_key
  ON production_plan_item (ppi_cpm_product_sys_id, ppi_machine_group_id, ppi_shade_code, ppi_deadline);
