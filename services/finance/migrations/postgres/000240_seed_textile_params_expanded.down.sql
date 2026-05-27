-- 000240 down: soft-delete all params seeded with created_by='seed_000240'.

BEGIN;

UPDATE mst_parameter
   SET is_active = FALSE,
       deleted_at = NOW(),
       deleted_by = 'seed_000240'
 WHERE created_by = 'seed_000240'
   AND deleted_at IS NULL;

COMMIT;
