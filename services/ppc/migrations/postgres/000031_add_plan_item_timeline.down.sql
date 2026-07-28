-- Reverse 000031. The month backfill is not reversible (the pre-backfill
-- divergent values are not recorded anywhere) and is intentionally left as-is:
-- month == to_char(deadline) is the correct state either way.

ALTER TABLE production_plan_item
  DROP CONSTRAINT IF EXISTS chk_ppi_duration_positive;

ALTER TABLE production_plan_item
  DROP CONSTRAINT IF EXISTS chk_ppi_duration_source;

ALTER TABLE production_plan_item
  DROP COLUMN IF EXISTS ppi_duration_source,
  DROP COLUMN IF EXISTS ppi_planned_duration_days,
  DROP COLUMN IF EXISTS ppi_planned_start_date;
