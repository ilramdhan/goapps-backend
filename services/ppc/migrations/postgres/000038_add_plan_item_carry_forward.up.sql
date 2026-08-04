-- 000038: plan-item carry-forward source link.
--
-- Carrying a plan item into a new month creates a NEW ROW rather than moving
-- the existing one's ppi_month. The alternative (reassignment) was rejected
-- because the source month's plan is a record of what was committed there: a
-- row that silently relocates makes last month's plan retroactively wrong, and
-- production_plan_log records field changes but not the row's own provenance.
-- A new row also keeps wo_plan_item_link intact — the work orders raised in the
-- source month still point at the plan item they were actually raised against.
--
-- ppi_carry_from_item_id is the self-reference that makes the new row traceable
-- to its source (S-2.2) and is also the double-carry guard: a second run over
-- the same source month finds the existing child and reports the candidate as
-- already carried instead of creating a duplicate.
--
-- Mirrors the demand-side pd_carry_from_id / pd_carry_action pair added in
-- 000011, using this table's ppi_ prefix.

ALTER TABLE production_plan_item
  ADD COLUMN IF NOT EXISTS ppi_carry_from_item_id BIGINT REFERENCES production_plan_item(ppi_id),
  ADD COLUMN IF NOT EXISTS ppi_carry_action       VARCHAR(20);

COMMENT ON COLUMN production_plan_item.ppi_carry_from_item_id IS
  'Source plan item this row was carried forward from at month start. NULL for an ordinary item. Self-referential, mirroring production_demand.pd_carry_from_id.';
COMMENT ON COLUMN production_plan_item.ppi_carry_action IS
  'Carry-forward action that produced this row: CARRY_AS_IS or PARTIAL_CARRY. NULL for an ordinary item.';

-- Backs the double-carry guard: "does a plan item in the target month already
-- name this source?" is asked once per candidate on every candidate list.
CREATE INDEX IF NOT EXISTS idx_ppi_carry_from_item_id
  ON production_plan_item (ppi_carry_from_item_id)
  WHERE ppi_carry_from_item_id IS NOT NULL;
