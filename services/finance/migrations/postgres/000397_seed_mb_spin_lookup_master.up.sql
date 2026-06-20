-- 000397: Register MB_SPIN as a lookup master so it appears in the admin registry.
BEGIN;

INSERT INTO mst_lookup_master (lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, lm_table_name, created_by)
VALUES ('MB_SPIN', 'MB Spin', '', '', '', 'mst_mb_spin', 'seed_000397')
ON CONFLICT (lm_code) DO NOTHING;

COMMIT;
