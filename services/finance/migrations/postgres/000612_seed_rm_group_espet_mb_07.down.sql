-- 000612 down: remove the seed_000612 RM group + its dummy item (children first).
-- Only rows created by this seed are touched.
BEGIN;
DELETE FROM cst_rm_group_detail WHERE created_by = 'seed_000612'
  AND group_head_id IN (SELECT group_head_id FROM cst_rm_group_head WHERE created_by = 'seed_000612');
DELETE FROM cst_rm_group_head WHERE created_by = 'seed_000612';
COMMIT;
