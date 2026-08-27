-- Reverse 000494 — drop the cpp_value_mb_spin_id companion column from
-- cost_product_parameter.
--
-- DESTRUCTIVE: any resolved mbs_id links written after this migration ran are
-- lost. cpp_value_text (and cpp_one_value_chk, which this migration never
-- touched) are unaffected and keep working exactly as before 000494.
--
-- Order: index -> constraint -> column, so a failure partway through is
-- informative. DROP COLUMN would already cascade-drop the index and
-- constraint, but each is dropped explicitly first, mirroring 000490's
-- down migration.

BEGIN;

DROP INDEX IF EXISTS idx_cpp_value_mb_spin_id;

ALTER TABLE cost_product_parameter
    DROP CONSTRAINT IF EXISTS fk_cpp_value_mb_spin;

ALTER TABLE cost_product_parameter
    DROP COLUMN IF EXISTS cpp_value_mb_spin_id;

COMMIT;
