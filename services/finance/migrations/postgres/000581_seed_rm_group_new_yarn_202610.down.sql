-- 000581 down: remove the 8 seed_000581 RM groups + their dummy items (children first).
-- Only rows created by this seed are touched.
BEGIN;
DELETE FROM cst_rm_group_detail WHERE created_by = 'seed_000581'
  AND group_head_id IN (SELECT group_head_id FROM cst_rm_group_head WHERE created_by = 'seed_000581');
DELETE FROM cst_rm_group_head WHERE created_by = 'seed_000581';
COMMIT;
