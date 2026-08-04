-- 000464 down: remove only what this migration seeded, and never a human's correction.
--
-- PRESERVING EDITS (the reason for the cpp_updated_by test below).
-- A costing user who corrects a seeded weight through the UI stamps cpp_updated_by and
-- leaves cpp_created_by alone. Scoping the delete on cpp_created_by ALONE would therefore
-- delete their correction too -- and because the up migration is idempotent, re-running it
-- would re-insert the original modal value. Down-then-up would silently REVERT a human
-- decision to a machine-derived one, with nothing in the schema recording that it happened.
-- So an edited row is kept. It is then a value whose definition is soft-deleted, which is
-- inert (nothing reads it) and fully recoverable -- strictly better than destroying it.
--
-- DEFINITION IS SOFT-DELETED, and the up migration is deliberately written to match.
-- A hard DELETE would be blocked by cost_product_parameter's FK for exactly the rows we
-- just chose to keep. Soft delete also matches how the parameter master is maintained
-- everywhere else in this service. The asymmetry this creates -- a later `up` would insert
-- a SECOND definition with a fresh id, orphaning the kept values against the old one -- is
-- closed by the RESURRECT step below: `up` reuses the soft-deleted row instead of creating
-- a new one, so a down/up cycle is idempotent at the id level and the kept values stay
-- attached to the definition they were written against.
BEGIN;

-- 1. Delete only untouched seed rows. cpp_updated_by IS NOT NULL marks a human edit.
DELETE FROM cost_product_parameter v
USING mst_parameter p
WHERE p.id = v.cpp_param_id
  AND p.param_code = 'STD_WEIGHT'
  AND v.cpp_created_by = 'seed_ppc_lot'
  AND v.cpp_updated_by IS NULL;

-- 2. Soft-delete the definition we created (never one somebody else created).
UPDATE mst_parameter
   SET deleted_at = now(), deleted_by = 'seed_ppc_lot', is_active = FALSE
 WHERE param_code = 'STD_WEIGHT'
   AND created_by = 'seed_ppc_lot'
   AND deleted_at IS NULL;

DO $$
DECLARE kept INTEGER; edited INTEGER;
BEGIN
  SELECT count(*) INTO kept
    FROM cost_product_parameter v
    JOIN mst_parameter p ON p.id = v.cpp_param_id
   WHERE p.param_code = 'STD_WEIGHT';
  SELECT count(*) INTO edited
    FROM cost_product_parameter v
    JOIN mst_parameter p ON p.id = v.cpp_param_id
   WHERE p.param_code = 'STD_WEIGHT'
     AND v.cpp_created_by = 'seed_ppc_lot'
     AND v.cpp_updated_by IS NOT NULL;
  IF kept > 0 THEN
    RAISE NOTICE '000464 down: % STD_WEIGHT value(s) kept (% of them hand-edited and deliberately preserved)', kept, edited;
  END IF;
END $$;

COMMIT;
