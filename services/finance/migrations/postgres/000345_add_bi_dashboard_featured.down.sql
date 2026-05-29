ALTER TABLE bi_dashboard
  DROP COLUMN IF EXISTS is_featured,
  DROP COLUMN IF EXISTS feature_order;
