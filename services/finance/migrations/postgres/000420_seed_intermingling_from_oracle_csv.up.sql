-- 000420: Seed mst_intermingling from Oracle CSV (CST_YARN_MST_INTERMINGLING).
-- Generated: 2026-06-26  Source rows: 19
-- No new schema columns needed.
-- CYMI_VALUE stored as-is (costing engine divides by 100 at lookup time).
-- NIM row has NULL value → stored as 0 (no intermingling cost).

BEGIN;

DELETE FROM public.mst_intermingling;

INSERT INTO public.mst_intermingling (
  intm_oracle_sys_id, intm_code, intm_name, intm_cost_per_kg,
  is_active, created_by, created_at, updated_by, updated_at
)
VALUES
  ('20201101', 'HIM', 'HIM', 6.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201102', 'HTDH HIM', 'HTDH HIM', 7.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201103', 'HCDH HIM', 'HCDH HIM', 7.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201104', 'HT HIM', 'HT HIM', 7.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201105', 'LTY HIM', 'LTY HIM', 7.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201106', 'LTH', 'LTH', 7.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201107', 'HCSH NIM', 'HCSH NIM', 1, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201108', 'LIM', 'LIM', 2.4, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201109', 'IM', 'IM', 5.44, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201110', 'HIMD', 'HIMD', 7.5, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201111', 'HT IM', 'HT IM', 6.44, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201112', 'IM BSY', 'IM BSY', 6.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201113', 'BSY', 'BSY', 6.8, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201114', 'NIM', 'NIM', 0, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201115', 'SIM', 'SIM', 3, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20201116', 'ANN', 'ANN', 7, TRUE, 'oracle_csv', NOW(), NULL, NULL),
  ('20250318', 'ACD-CA(1100-2500)', 'ACD-CA(1100-2500)', 10.72, TRUE, 'SINTIA', '2025-03-03 11:51:52', NULL, NULL),
  ('20250319', 'ARD-CA(600-1100)', 'ARD-CA(600-1100)', 10.13, TRUE, 'SINTIA', '2025-03-03 11:51:52', NULL, NULL),
  ('20250320', 'AMD-CA(250-600)', 'AMD-CA(250-600)', 8.26, TRUE, 'SINTIA', '2025-03-03 11:51:52', NULL, NULL);

DO $$
DECLARE v_inserted INTEGER;
BEGIN
  SELECT COUNT(*) INTO v_inserted FROM public.mst_intermingling WHERE deleted_at IS NULL;
  RAISE NOTICE '000420: mst_intermingling inserted=%  (expected=19)', v_inserted;
END $$;

COMMIT;