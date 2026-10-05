-- 000559 down — drop the backtest report lines.
BEGIN;
DROP TABLE IF EXISTS cst_erp_backtest_line;
COMMIT;
