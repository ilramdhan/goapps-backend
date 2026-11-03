-- P002 (DOWN): hapus registry PACK_*.
DELETE FROM bi_metric_registry
WHERE metric_name IN ('PACK_TOTAL', 'PACK_AX', 'PACK_FQ', 'PACK_DG', 'PACK_JLT');
