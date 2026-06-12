ALTER TABLE cost_product_request
  ADD COLUMN IF NOT EXISTS cpr_wfl_instance_id UUID;

COMMENT ON COLUMN cost_product_request.cpr_wfl_instance_id IS
  'Pointer to IAM wfl_workflow_instance.id (no cross-DB FK). Set on Submit().';
