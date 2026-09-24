-- 000522 (oil-cost-rm-group M3) — RM_GROUP_OIL lookup master.
--
-- v_rm_group_oil is the single definition of "oil group": active, flagged RM
-- group heads. It exposes deleted_at because the generic ListMasterOptions
-- query (lookup_master_repository.go) always appends "WHERE deleted_at IS NULL".
-- Registering the VIEW as lm_table_name lets ListMasterOptions, the import
-- existence validation (cost_import_master_lookup.go) and the registry admin
-- screens work unchanged (decision D11).
--
-- OIL_NAME stores group_code (value) and shows group_name (label).
--
-- Idempotent: CREATE OR REPLACE VIEW, registry INSERT ON CONFLICT DO NOTHING.

BEGIN;

CREATE OR REPLACE VIEW v_rm_group_oil AS
SELECT h.group_code,
       h.group_name,
       h.group_head_id,
       h.deleted_at
FROM cst_rm_group_head h
WHERE h.is_oil_group
  AND h.is_active;

COMMENT ON VIEW v_rm_group_oil IS
    'Oil RM groups (is_oil_group AND is_active). Source table of lookup master RM_GROUP_OIL (OIL_NAME options).';

INSERT INTO mst_lookup_master (
    lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field,
    lm_table_name, created_by
)
VALUES (
    'RM_GROUP_OIL', 'RM Group (Oil)', '/api/v1/finance/rm-groups', 'group_code', 'group_name',
    'v_rm_group_oil', 'seed_000522'
)
ON CONFLICT (lm_code) DO NOTHING;

COMMIT;
