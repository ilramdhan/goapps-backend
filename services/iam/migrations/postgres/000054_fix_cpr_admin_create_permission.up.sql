-- Migration 000054: Remove finance.product.request.create from CPR_ADMIN role.
-- CPR_ADMIN manages fill configs, reviews, and reopens requests but should NOT
-- create new requests (that is CPR_REQUESTER's job). Having this permission
-- caused the "+ New Request" button to appear for all CPR_ADMIN users (finance01,
-- financemgr, productionmgr).
DELETE FROM role_permissions
WHERE role_id  = (SELECT role_id FROM mst_role WHERE role_code = 'CPR_ADMIN')
  AND permission_id = (
    SELECT permission_id FROM mst_permission
    WHERE permission_code = 'finance.product.request.create'
  );
