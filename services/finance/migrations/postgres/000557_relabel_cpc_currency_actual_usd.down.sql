-- 000557 down — restore the labels of exactly the rows 000557 relabelled
-- (from cst_currency_relabel_log), then drop the log. Rows not in the log
-- (e.g. new ACTUAL rows written as USD by the fixed writer, or 202605) are
-- never touched. Values are never touched.

BEGIN;

UPDATE cst_product_cost c
   SET cpc_currency_code = l.ccrl_old_label
  FROM cst_currency_relabel_log l
 WHERE l.ccrl_migration = 'migration:000557'
   AND l.ccrl_cpc_cost_id = c.cpc_cost_id
   AND c.cpc_currency_code = l.ccrl_new_label;

DROP TABLE IF EXISTS cst_currency_relabel_log;

COMMIT;
