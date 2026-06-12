-- Add INACTIVE status to the fill-task status constraint.
-- INACTIVE tasks are completion-chain tasks (L101, L102) that have been created but
-- not yet activated; they become ACTIVE when the preceding level is approved.
ALTER TABLE cost_fill_task
  DROP CONSTRAINT IF EXISTS chk_cft_status;

ALTER TABLE cost_fill_task
  ADD CONSTRAINT chk_cft_status CHECK (cft_status IN
    ('INACTIVE','ACTIVE','FILLING','FILLED','APPROVAL_PENDING','APPROVED','REJECTED'));
