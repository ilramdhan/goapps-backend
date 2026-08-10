-- Reverse 000470. Everything is keyed on the marker, so no pre-existing row is
-- touched. CPP values go first — they are the leaf rows; the CAPP checklist is
-- what the engine gates on, so removing it last keeps the state consistent if the
-- transaction is inspected mid-flight.

BEGIN;

DELETE FROM cost_product_parameter
WHERE cpp_created_by = 'backfill_group_c_000470';

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'backfill_group_c_000470';

COMMIT;
