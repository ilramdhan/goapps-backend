-- Reverse 000537: remove only the mappings this seed inserted (the types fall
-- back to the ratio formula again). Guarded when 000536 was already reversed.
BEGIN;

DO $$
DECLARE
    v_deleted INT;
BEGIN
    IF to_regclass('public.mst_yarn_tx_weight_group_type') IS NULL THEN
        RAISE NOTICE '000537 down: mst_yarn_tx_weight_group_type absent — nothing to reverse.';
        RETURN;
    END IF;
    DELETE FROM mst_yarn_tx_weight_group_type WHERE created_by = 'seed_000537';
    GET DIAGNOSTICS v_deleted = ROW_COUNT;
    RAISE NOTICE '000537 down: removed % seeded mapping(s).', v_deleted;
END $$;

COMMIT;
