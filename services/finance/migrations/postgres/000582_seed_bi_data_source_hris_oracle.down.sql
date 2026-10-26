-- H005 (DOWN): hapus data source HRIS jika belum dipakai job/upload/fakta.
DELETE FROM bi_data_source
WHERE source_code = 'HRIS_ORACLE'
  AND NOT EXISTS (SELECT 1 FROM bi_job j WHERE j.source_id = bi_data_source.source_id)
  AND NOT EXISTS (SELECT 1 FROM bi_excel_upload u WHERE u.source_id = bi_data_source.source_id)
  AND NOT EXISTS (SELECT 1 FROM bi_fact_metric f WHERE f.source_id = bi_data_source.source_id);
