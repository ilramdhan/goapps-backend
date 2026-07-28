-- 000031: hybrid timeline on production_plan_item + month/deadline reconciliation.
--
-- Gantt bars previously had no real start: the handler faked
-- `start = deadline - 1 day`, so every bar was one day wide. Store the planned
-- start and duration explicitly. The system derives a default duration from
-- qty/capacity; a planner may override it, which flips ppi_duration_source to
-- MANUAL so later qty edits leave the override alone.
--
-- Month is a derived projection of the deadline (YYYY-MM). Rows where the two
-- diverged rendered outside their own month band, so backfill them here.

ALTER TABLE production_plan_item
  ADD COLUMN IF NOT EXISTS ppi_planned_start_date DATE,
  ADD COLUMN IF NOT EXISTS ppi_planned_duration_days INT,
  ADD COLUMN IF NOT EXISTS ppi_duration_source VARCHAR(10) NOT NULL DEFAULT 'DERIVED';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_ppi_duration_source'
  ) THEN
    ALTER TABLE production_plan_item
      ADD CONSTRAINT chk_ppi_duration_source
      CHECK (ppi_duration_source IN ('DERIVED', 'MANUAL'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_ppi_duration_positive'
  ) THEN
    ALTER TABLE production_plan_item
      ADD CONSTRAINT chk_ppi_duration_positive
      CHECK (ppi_planned_duration_days IS NULL OR ppi_planned_duration_days >= 1);
  END IF;
END
$$;

COMMENT ON COLUMN production_plan_item.ppi_planned_start_date IS
  'Planned production start. NULL falls back to the deadline when drawing Gantt bars.';
COMMENT ON COLUMN production_plan_item.ppi_planned_duration_days IS
  'Inclusive span in days between planned start and deadline. Clamped to [1,60] on derivation.';
COMMENT ON COLUMN production_plan_item.ppi_duration_source IS
  'DERIVED = recomputed from qty/capacity on every qty edit; MANUAL = planner override, never recomputed.';

-- Month is derived from the deadline; correct any row where it diverged.
UPDATE production_demand
   SET pd_month = to_char(pd_deadline, 'YYYY-MM')
 WHERE pd_month <> to_char(pd_deadline, 'YYYY-MM');

UPDATE production_plan_item
   SET ppi_month = to_char(ppi_deadline, 'YYYY-MM')
 WHERE ppi_month <> to_char(ppi_deadline, 'YYYY-MM');
