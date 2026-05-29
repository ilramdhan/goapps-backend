BEGIN;
UPDATE bi_fact_metric SET uom = 'PCS' WHERE type='SALES' AND metric_name='QUANTITY' AND uom='KGS';
UPDATE bi_metric_registry SET uom = 'PCS' WHERE metric_name='QUANTITY';
COMMIT;
