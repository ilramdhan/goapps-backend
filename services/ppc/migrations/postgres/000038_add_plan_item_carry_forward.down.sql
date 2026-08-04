-- Reverse 000038. Dropping ppi_carry_from_item_id discards the provenance of
-- every carried plan item; the rows themselves survive as ordinary plan items
-- in their target month, so no production commitment is lost.

DROP INDEX IF EXISTS idx_ppi_carry_from_item_id;

ALTER TABLE production_plan_item
  DROP COLUMN IF EXISTS ppi_carry_action,
  DROP COLUMN IF EXISTS ppi_carry_from_item_id;
