-- Data engineer confirmed: QUANTITY metric for Delivery Margin uses KGS (kilograms), not PCS.
-- Future Oracle MVs will deliver KGS natively; this corrects the existing Excel seed data.
BEGIN;

UPDATE bi_fact_metric
  SET uom = 'KGS'
WHERE type = 'SALES'
  AND metric_name = 'QUANTITY'
  AND uom = 'PCS';

UPDATE bi_metric_registry
  SET uom = 'KGS'
WHERE metric_name = 'QUANTITY';

COMMIT;
