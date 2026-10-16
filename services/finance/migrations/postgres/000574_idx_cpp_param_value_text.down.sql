-- 000574 down: drop the master-lookup-sync key-text index.
BEGIN;
SET LOCAL lock_timeout = '5s';
DROP INDEX IF EXISTS idx_cpp_param_value_text;
COMMIT;
