-- Revert: remove INACTIVE from the fill-task status constraint.
-- Note: this will fail if any rows have cft_status = 'INACTIVE'.
ALTER TABLE cost_fill_task
  DROP CONSTRAINT IF EXISTS chk_cft_status;

ALTER TABLE cost_fill_task
  ADD CONSTRAINT chk_cft_status CHECK (cft_status IN
    ('ACTIVE','FILLING','FILLED','APPROVAL_PENDING','APPROVED','REJECTED'));
