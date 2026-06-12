-- Migration 000052 rollback: remove section assignments from test users
UPDATE mst_user_detail
SET section_id = NULL
WHERE user_id IN (
  SELECT user_id FROM mst_user
  WHERE username IN (
    'finance01', 'financemgr',
    'production01', 'production02', 'production03', 'productionmgr',
    'marketing01', 'marketingmgr'
  ) AND deleted_at IS NULL
);
