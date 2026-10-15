-- 000573 down — restore OPU values changed by 000573 from the backup table.

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TABLE IF NOT EXISTS cost_product_parameter_opu_backup_573 (
    cpp_product_sys_id BIGINT      PRIMARY KEY,
    had_row            BOOLEAN     NOT NULL,
    old_value          NUMERIC,
    backed_up_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Rows that did not exist before: delete (only if still the value we set).
DELETE FROM cost_product_parameter p
USING cost_product_parameter_opu_backup_573 b, mst_parameter mp
WHERE b.cpp_product_sys_id = p.cpp_product_sys_id
  AND NOT b.had_row
  AND mp.id = p.cpp_param_id AND mp.param_code = 'OPU' AND mp.deleted_at IS NULL
  AND p.cpp_created_by = 'migration_000573';

-- Rows that existed: restore old value.
UPDATE cost_product_parameter p
SET cpp_value_numeric = b.old_value
FROM cost_product_parameter_opu_backup_573 b, mst_parameter mp
WHERE b.cpp_product_sys_id = p.cpp_product_sys_id
  AND b.had_row
  AND mp.id = p.cpp_param_id AND mp.param_code = 'OPU' AND mp.deleted_at IS NULL
  AND p.cpp_value_numeric IS DISTINCT FROM b.old_value;

DROP TABLE cost_product_parameter_opu_backup_573;

COMMIT;
