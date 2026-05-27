-- 000244.down: drop only the routes seeded by 000244.
-- Identified by crh_created_by = 'seed_000244'.

BEGIN;

DELETE FROM cost_route_rm
 WHERE crm_seq_id IN (
    SELECT crs.crs_seq_id
      FROM cost_route_seq crs
      JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
     WHERE crh.crh_created_by = 'seed_000244'
 );

DELETE FROM cost_route_seq
 WHERE crs_head_id IN (
    SELECT crh_head_id FROM cost_route_head
     WHERE crh_created_by = 'seed_000244'
 );

DELETE FROM cost_route_head
 WHERE crh_created_by = 'seed_000244';

COMMIT;
