BEGIN;
DROP INDEX IF EXISTS idx_crh_forked_from;
ALTER TABLE cost_route_head DROP COLUMN IF EXISTS crh_forked_from_head_id;
COMMIT;
