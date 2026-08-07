-- Reverse 000468: drop the 6 Group-C params. The wiring lives on those same
-- rows (lookup_master_code / lookup_fill_group_code / lookup_source_column), so
-- deleting them removes it too. No pre-existing param was modified.
BEGIN;

DELETE FROM mst_parameter WHERE created_by = 'wire_group_c_000468';

COMMIT;
