-- Reverse 000032. Dropping the columns discards any pulled shade values; a
-- re-pull from Orion staging restores them.

DROP INDEX IF EXISTS idx_ppi_merge_key;

ALTER TABLE production_plan_item
  DROP COLUMN IF EXISTS ppi_shade_name,
  DROP COLUMN IF EXISTS ppi_shade_code;

ALTER TABLE production_demand
  DROP COLUMN IF EXISTS pd_shade_name,
  DROP COLUMN IF EXISTS pd_shade_code;
