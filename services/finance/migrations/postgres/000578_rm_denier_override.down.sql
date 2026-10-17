-- 000578 down — drop the RM denier override table and the duplicate link
-- column. Both are new in 000578 (no pre-existing data to restore).

BEGIN;

SET LOCAL lock_timeout = '10s';

DROP TABLE IF EXISTS cost_product_rm_denier_override;

DROP INDEX IF EXISTS idx_cpm_duplicated_from;

ALTER TABLE cost_product_master DROP COLUMN IF EXISTS cpm_duplicated_from_sys_id;

COMMIT;
