-- Rollback Phase C (Calc Engine) S8a.7 permissions seed.
-- Note: the previous chk_permission_action constraint is NOT restored; restore
-- manually if a true rollback is required.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'finance.cost.%'
);

DELETE FROM mst_permission WHERE permission_code LIKE 'finance.cost.%';

COMMIT;
