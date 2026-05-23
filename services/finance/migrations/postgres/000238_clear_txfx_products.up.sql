-- 000238: Clear the TXFX_* products, routes, CAPP, and CPP seeded by 000236.
-- We are about to re-seed in 000239 with a much deeper DAG (10-12 levels deep)
-- and multi-stage routes. Easiest path is to wipe the existing TXFX fixture and
-- recreate clean -- safer than trying to amend in-place because route_seq +
-- route_rm structure changes (multi-seq routes per product).
--
-- Safety: every WHERE clause is filtered to TXFX_% codes only, so this cannot
-- touch real production product / route data.

BEGIN;

-- Route RMs (children of route seqs of TXFX products).
DELETE FROM cost_route_rm
 WHERE crm_seq_id IN (
   SELECT crs.crs_seq_id
     FROM cost_route_seq crs
     JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
     JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
    WHERE cpm.cpm_product_code LIKE 'TXFX_%'
 );

-- Route seqs.
DELETE FROM cost_route_seq
 WHERE crs_head_id IN (
   SELECT crh.crh_head_id
     FROM cost_route_head crh
     JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
    WHERE cpm.cpm_product_code LIKE 'TXFX_%'
 );

-- Route heads.
DELETE FROM cost_route_head
 WHERE crh_product_sys_id IN (
   SELECT cpm_product_sys_id FROM cost_product_master
    WHERE cpm_product_code LIKE 'TXFX_%'
 );

-- Product param values + applicability (defensive: CASCADE off cpm would also
-- catch these, but be explicit in case schema changes).
DELETE FROM cost_product_parameter
 WHERE cpp_product_sys_id IN (
   SELECT cpm_product_sys_id FROM cost_product_master
    WHERE cpm_product_code LIKE 'TXFX_%'
 );

DELETE FROM cost_product_applicable_param
 WHERE capp_product_sys_id IN (
   SELECT cpm_product_sys_id FROM cost_product_master
    WHERE cpm_product_code LIKE 'TXFX_%'
 );

-- Finally the product master rows themselves.
DELETE FROM cost_product_master
 WHERE cpm_product_code LIKE 'TXFX_%';

COMMIT;
