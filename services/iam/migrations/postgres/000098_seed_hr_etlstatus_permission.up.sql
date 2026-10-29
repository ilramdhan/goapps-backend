-- H062b / HD-27: permission status ETL HR (RPC BiJobService.GetEtlStatus, banner "data per run terakhir").
-- Diberikan ke HR & DIREKSI_GM (hanya role bertanda '[H004]' dari 000096) dan SUPER_ADMIN. Idempotent.
BEGIN;

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.bi.etlstatus.view', 'View BI ETL Status', 'Lihat status run terakhir job ETL HR (tanpa error/config)', 'finance', 'bi', 'view', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code IN ('DIREKSI_GM', 'HR') AND r.description LIKE '[H004]%' AND r.is_active = true
  AND p.permission_code = 'finance.bi.etlstatus.view' AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = true
  AND p.permission_code = 'finance.bi.etlstatus.view' AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
