-- P002 / ETL§E: registry metric Packed Production. Idempotent.
BEGIN;

INSERT INTO bi_metric_registry (metric_name, display_label, metric_category, agg_method, uom, description, number_format)
VALUES ('PACK_TOTAL', 'Packed Total', 'VOLUME', 'SUM', 'KGS', 'Packed production semua grade (Kgs)', 'integer_thousands'),
       ('PACK_AX',    'Packed AX',    'VOLUME', 'SUM', 'KGS', 'Packed production grade AX (Kgs)', 'integer_thousands'),
       ('PACK_FQ',    'Packed FQ',    'VOLUME', 'SUM', 'KGS', 'Packed production first quality (Kgs)', 'integer_thousands'),
       ('PACK_DG',    'Packed DG',    'VOLUME', 'SUM', 'KGS', 'Packed production downgrade (Kgs)', 'integer_thousands'),
       ('PACK_JLT',   'Packed JLT',   'VOLUME', 'SUM', 'KGS', 'Packed production grade JLT (Kgs)', 'integer_thousands')
ON CONFLICT (metric_name) DO NOTHING;

COMMIT;
