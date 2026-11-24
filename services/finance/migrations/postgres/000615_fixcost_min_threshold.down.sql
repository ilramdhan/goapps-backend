-- 000615 down: remove the fixed cost floor columns and the injected param.
BEGIN;

SET LOCAL lock_timeout = '10s';

DELETE FROM mst_parameter
 WHERE param_code = 'FIXEDCOST_SHORTFALL'
   AND created_by = 'fixcost_min_threshold_000615';

ALTER TABLE cost_product_master DROP CONSTRAINT IF EXISTS chk_cpm_fixcost_min_threshold;
ALTER TABLE mst_machine         DROP CONSTRAINT IF EXISTS chk_mc_fixcost_min_threshold;
ALTER TABLE cost_product_master DROP COLUMN IF EXISTS cpm_fixcost_min_threshold;
ALTER TABLE mst_machine         DROP COLUMN IF EXISTS mc_fixcost_min_threshold;

COMMIT;
