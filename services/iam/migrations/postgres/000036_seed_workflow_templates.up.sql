-- Seed default workflow templates (PRODUCT_COSTING 3-step + PARAM_FILL 1-step).
-- PRD v1.3 §0.1 D6.

INSERT INTO wfl_workflow_template (template_id, kind, name, version, is_active, description, created_by)
VALUES
    ('00000000-0000-0000-0000-00000000c001', 'PRODUCT_COSTING', 'Default Product Costing Workflow', 1, TRUE, 'Finance -> Abhisek -> Basker', 'system'),
    ('00000000-0000-0000-0000-00000000c002', 'PARAM_FILL',      'Default Param Fill Workflow',     1, TRUE, 'Department owner approval',  'system')
ON CONFLICT (template_id) DO NOTHING;

INSERT INTO wfl_workflow_template_step
    (template_id, step_no, step_name, approver_resolution_type, approver_resolution_value, allow_reject, allow_reassign, require_password_on_unlock, created_by)
VALUES
    ('00000000-0000-0000-0000-00000000c001', 1, 'Finance Review',  'ROLE', 'FINANCE',       TRUE, FALSE, FALSE, 'system'),
    ('00000000-0000-0000-0000-00000000c001', 2, 'Abhisek Review',  'USER', 'abhisek',       TRUE, TRUE,  FALSE, 'system'),
    ('00000000-0000-0000-0000-00000000c001', 3, 'Basker Lock',     'USER', 'basker',        TRUE, TRUE,  TRUE,  'system'),
    ('00000000-0000-0000-0000-00000000c002', 1, 'Dept Owner Fill', 'DEPT', 'PARAM_OWNER',   TRUE, FALSE, FALSE, 'system')
ON CONFLICT (template_id, step_no) DO NOTHING;
