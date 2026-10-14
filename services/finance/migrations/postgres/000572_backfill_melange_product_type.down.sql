-- 000572 down — restore the melange products' previous product type and
-- undo MEL's oil class / oil-group mapping set by 000572.

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TABLE IF NOT EXISTS cost_product_type_backfill_572 (
    cpm_product_sys_id BIGINT      PRIMARY KEY,
    old_type_id        INT         NOT NULL,
    backed_up_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS cost_product_type_backfill_572_meta (
    key   VARCHAR(50) PRIMARY KEY,
    value TEXT
);

UPDATE cost_product_master pm
SET cpm_product_type_id = b.old_type_id,
    cpm_updated_at      = NOW(),
    cpm_updated_by      = 'migration_000572_down'
FROM cost_product_type_backfill_572 b
WHERE pm.cpm_product_sys_id = b.cpm_product_sys_id;

-- Oil CAPP rows added by 000572 (values users may have entered on them in
-- cost_product_parameter are kept; they are inert without the CAPP row).
DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'migration_000572';

DELETE FROM cost_product_type_oil_group
WHERE cptog_created_by = 'migration_000572';

UPDATE cost_product_type
SET cpt_oil_class  = NULL,
    cpt_updated_at = NOW()
WHERE cpt_type_code = 'MEL'
  AND EXISTS (
      SELECT 1 FROM cost_product_type_backfill_572_meta
      WHERE key = 'mel_oil_class_set' AND value = 'true'
  );

DROP TABLE cost_product_type_backfill_572;
DROP TABLE cost_product_type_backfill_572_meta;

COMMIT;
