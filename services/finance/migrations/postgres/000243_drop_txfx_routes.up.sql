-- 000243: Drop all TXFX_* routes (heads + seqs + rms) prior to re-seed in 000244.
--
-- Why: the 000239 seed created one route_head per product (FG + every
-- intermediate) where every seq inside a route had the same
-- crs_product_sys_id = head product. That's the wrong mental model:
--   * crs_product_sys_id should be the SPECIFIC product produced at that seq
--   * a route_head should represent the FG and its INTERNAL DAG, where each
--     intermediate stage is its own seq with its own product, and PRODUCT-RMs
--     link seqs to upstream seqs within the same route
--
-- This migration drops every TXFX_* route_head (FG + intermediate) so 000244
-- can re-seed exactly 8 FG-rooted multi-product DAGs.
--
-- Pre-check: cost_product_request.cpr_linked_route_head_id, cst_product_cost
-- and cal_job_product currently have ZERO references to any TXFX route_head,
-- so the cascade is safe.

BEGIN;

DELETE FROM cost_route_rm
 WHERE crm_seq_id IN (
    SELECT crs.crs_seq_id
      FROM cost_route_seq crs
      JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
      JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
     WHERE cpm.cpm_product_code LIKE 'TXFX\_%' ESCAPE '\'
 );

DELETE FROM cost_route_seq
 WHERE crs_head_id IN (
    SELECT crh.crh_head_id
      FROM cost_route_head crh
      JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
     WHERE cpm.cpm_product_code LIKE 'TXFX\_%' ESCAPE '\'
 );

DELETE FROM cost_route_head
 WHERE crh_product_sys_id IN (
    SELECT cpm_product_sys_id FROM cost_product_master
     WHERE cpm_product_code LIKE 'TXFX\_%' ESCAPE '\'
 );

COMMIT;
