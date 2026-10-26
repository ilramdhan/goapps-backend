-- H005 / HD-02: data source Oracle HRIS (read-only account BI_HR_RO), dipisah dari ERP_ORACLE (K5).
-- Idempotent.
INSERT INTO bi_data_source (source_code, source_name, source_type, description, created_at)
VALUES ('HRIS_ORACLE', 'Oracle HRIS (MGTHRIS)', 'ORACLE', 'HRIS read-only (BI_HR_RO) untuk HR Dashboard Workforce', NOW())
ON CONFLICT (source_code) DO NOTHING;
