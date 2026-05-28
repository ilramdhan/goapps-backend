-- Seed: 3 default data sources for BI module.
BEGIN;

INSERT INTO bi_data_source (source_code, source_name, source_type, description, created_at)
VALUES
  ('ERP_ORACLE',   'Oracle ERP 11g',        'ORACLE', 'Production ERP — placeholder pending data engineer', NOW()),
  ('LARAVEL_DB',   'Legacy Laravel Oracle', 'ORACLE', 'Laravel app on Oracle 11g',                          NOW()),
  ('EXCEL_UPLOAD', 'Excel Manual Upload',   'EXCEL',  'User-driven file ingestion',                         NOW())
ON CONFLICT (source_code) DO NOTHING;

COMMIT;
