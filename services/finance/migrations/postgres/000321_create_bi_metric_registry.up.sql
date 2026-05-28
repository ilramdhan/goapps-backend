-- Centralized metric metadata: display labels, UOM, number format.
-- API JOINs this table to build human-readable labels without embedding them in fact data.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_metric_registry (
  registry_id      BIGSERIAL    PRIMARY KEY,
  metric_name      VARCHAR(50)  UNIQUE NOT NULL,
  display_label    VARCHAR(100) NOT NULL,
  metric_category  VARCHAR(20)  NOT NULL,
  agg_method       VARCHAR(20)  NOT NULL,
  uom              VARCHAR(20),
  description      TEXT,
  number_format    VARCHAR(50)  NOT NULL DEFAULT 'currency_thousands',
  is_active        BOOLEAN      NOT NULL DEFAULT TRUE,
  created_at       TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- Seed: MIS metric (EBITDA / Net Profit pattern — single value per dimension combo).
INSERT INTO bi_metric_registry
  (metric_name, display_label, metric_category, agg_method, uom, description, number_format)
VALUES
  ('VALUE', 'Value', 'VALUE', 'SUM', 'USD', 'Generic single-metric (EBITDA/P&L pattern)', 'currency_thousands')
ON CONFLICT (metric_name) DO NOTHING;

-- Seed: SALES metrics (Delivery Margin pattern — 6 metrics per dimension combo).
INSERT INTO bi_metric_registry
  (metric_name, display_label, metric_category, agg_method, uom, description, number_format)
VALUES
  ('QUANTITY',     'Quantity',        'VOLUME', 'SUM', 'PCS', 'Volume in pieces',                                  'integer_thousands'),
  ('GROSS_SALES',  'Gross Sales',     'VALUE',  'SUM', 'USD', 'Total sales before deductions',                     'currency_thousands'),
  ('SELLING_COST', 'Selling Cost',    'VALUE',  'SUM', 'USD', 'Direct selling cost (commission, freight)',          'currency_thousands'),
  ('NETT_SALES',   'Net Sales',       'VALUE',  'SUM', 'USD', 'Gross sales minus selling cost',                    'currency_thousands'),
  ('COST_PROD',    'Production Cost', 'VALUE',  'SUM', 'USD', 'Cost of goods sold',                                'currency_thousands'),
  ('MARGIN',       'Margin',          'VALUE',  'SUM', 'USD', 'Net sales minus production cost (absolute margin)', 'currency_thousands')
ON CONFLICT (metric_name) DO NOTHING;

COMMIT;
