-- Revert 000033: restore the NOT NULL anchor on work_order.wo_plan_item_id.
--
-- Decision: BACKFILL rather than fail. Any work order whose anchor is NULL is
-- given the lowest-numbered plan item linked to it, so the down migration
-- succeeds on merged data instead of aborting. The merge itself is NOT undone:
-- wo_plan_item_link keeps every row, so re-running the up migration restores
-- the previous state exactly.
--
-- The only case that can still fail is a work order with a NULL anchor AND no
-- rows in wo_plan_item_link. The domain forbids that (a WO must have at least
-- one linked plan item), so it should not exist; if it does, the ALTER below
-- fails loudly rather than inventing a value, which is the correct outcome.

DROP INDEX IF EXISTS uq_wpl_plan_item;

UPDATE work_order wo
SET wo_plan_item_id = (
    SELECT MIN(l.wpl_plan_item_id)
    FROM wo_plan_item_link l
    WHERE l.wpl_wo_id = wo.wo_id
)
WHERE wo.wo_plan_item_id IS NULL;

ALTER TABLE work_order ALTER COLUMN wo_plan_item_id SET NOT NULL;

COMMENT ON COLUMN work_order.wo_plan_item_id IS NULL;
