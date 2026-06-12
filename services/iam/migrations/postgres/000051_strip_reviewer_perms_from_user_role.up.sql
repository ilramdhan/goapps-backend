-- Migration 000051: Strip CPR reviewer/admin-only permissions from USER role
--
-- The USER role inadvertently has finance.product.request.reject/resolve/assign
-- which are CPR-reviewer-only actions. This causes any authenticated user to see
-- Reject and Decide-Feasibility buttons regardless of their CPR role.
--
-- Remove the CPR-workflow permissions that belong exclusively to CPR_REVIEWER /
-- CPR_ADMIN from the base USER role. CPR_REQUESTER/CPR_SUBMITTER also have
-- .create and .view which remain in USER for backward compat.

BEGIN;

DELETE FROM role_permissions
WHERE role_id = (SELECT role_id FROM mst_role WHERE role_code = 'USER' LIMIT 1)
  AND permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
      'finance.product.request.reject',
      'finance.product.request.resolve',
      'finance.product.request.assign'
    )
  );

COMMIT;
