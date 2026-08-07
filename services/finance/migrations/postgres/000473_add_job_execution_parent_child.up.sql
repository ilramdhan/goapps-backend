-- Migration: Add parent/child batch tracking to job_execution.
-- Context: T8.2 bulk export at scale (fan-out). request_export_handler.go
-- currently silently truncates the product cost sheet export list to
-- maxExportProducts=200 when a filter resolves to more. Real periods can
-- resolve to 40,000+ products across 3 calc types, requiring 200+ separate
-- 200-product export jobs to cover fully. We mirror the existing
-- cal_job/cal_job_chunk parent/child pattern (000229/000230) but on
-- job_execution itself, since product_cost_sheet_export already lives there
-- (000471) and cloning a whole second tracking table pair would duplicate
-- job_execution's status/progress/notification machinery for no benefit.
--
-- jex_parent_job_id NULL means "not a child" — either a standalone job (the
-- existing single-job path, unchanged) or a parent/batch-tracking job (no
-- product list / file output of its own; exists purely to aggregate child
-- progress and fire exactly one completion notification).

ALTER TABLE job_execution
    ADD COLUMN IF NOT EXISTS jex_parent_job_id UUID REFERENCES job_execution(job_id),
    ADD COLUMN IF NOT EXISTS jex_total_children INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS jex_completed_children INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS jex_failed_children INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN job_execution.jex_parent_job_id IS
'Self-referencing FK to job_execution.job_id. NULL for standalone jobs and for
parent/batch-tracking jobs; set on every child job spawned by a batch fan-out.';
COMMENT ON COLUMN job_execution.jex_total_children IS
'Number of child jobs this parent expects to complete. 0 for non-parent rows.';
COMMENT ON COLUMN job_execution.jex_completed_children IS
'Number of child jobs that finished with SUCCESS, incremented atomically by
the worker as each child completes.';
COMMENT ON COLUMN job_execution.jex_failed_children IS
'Number of child jobs that finished with FAILED, incremented atomically by
the worker as each child completes.';

CREATE INDEX IF NOT EXISTS idx_job_execution_parent_job_id
    ON job_execution(jex_parent_job_id)
    WHERE jex_parent_job_id IS NOT NULL;

-- idx_job_execution_active_unique (000008) prevents duplicate ACTIVE jobs of
-- the same (job_type, period). Every child job in a batch fan-out shares
-- job_type='product_cost_sheet_export' and the same period as its siblings,
-- so the original index would collide on the second child's insert. Narrow
-- it to standalone/parent jobs only; children opt out via jex_parent_job_id
-- IS NULL. Children racing each other is fine — they are meant to run
-- concurrently.
DROP INDEX IF EXISTS idx_job_execution_active_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_job_execution_active_unique
    ON job_execution(job_type, period)
    WHERE status IN ('QUEUED', 'PROCESSING') AND jex_parent_job_id IS NULL;
