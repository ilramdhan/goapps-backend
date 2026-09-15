-- Reverse 000514. Everything is keyed on this migration's own marker
-- ('rebackfill_group_c_000514'), so no pre-existing row is touched — in
-- particular, rows written by 000470 under 'backfill_group_c_000470' (if that
-- migration's DML ever did land in some environment) survive untouched.
--
-- CPP values go first — they are the leaf rows; the CAPP checklist is what the
-- engine gates on, so removing it last keeps the state consistent if the
-- transaction is inspected mid-flight. Mirrors 000470_backfill_group_c_capp.down.sql.

BEGIN;

DELETE FROM cost_product_parameter
WHERE cpp_created_by = 'rebackfill_group_c_000514';

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'rebackfill_group_c_000514';

COMMIT;
