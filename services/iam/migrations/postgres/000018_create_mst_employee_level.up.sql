-- Create Employee Level master table
CREATE TABLE IF NOT EXISTS mst_employee_level (
    employee_level_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Core fields
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,
    grade SMALLINT NOT NULL DEFAULT 0,
    type SMALLINT NOT NULL DEFAULT 0,
    sequence SMALLINT NOT NULL DEFAULT 0,
    workflow SMALLINT NOT NULL DEFAULT 0,

    -- Status
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Audit trail
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),

    -- Soft delete
    deleted_at TIMESTAMPTZ,
    deleted_by VARCHAR(100)
);

-- Partial unique index on code (only non-deleted records)
CREATE UNIQUE INDEX IF NOT EXISTS idx_mst_employee_level_code
    ON mst_employee_level(code) WHERE deleted_at IS NULL;

-- Index for active filter
CREATE INDEX IF NOT EXISTS idx_mst_employee_level_active
    ON mst_employee_level(is_active) WHERE deleted_at IS NULL;

-- Index for sequence sorting
CREATE INDEX IF NOT EXISTS idx_mst_employee_level_sequence
    ON mst_employee_level(sequence) WHERE deleted_at IS NULL;

-- Index for type filter
CREATE INDEX IF NOT EXISTS idx_mst_employee_level_type
    ON mst_employee_level(type) WHERE deleted_at IS NULL;

-- Index for workflow filter
CREATE INDEX IF NOT EXISTS idx_mst_employee_level_workflow
    ON mst_employee_level(workflow) WHERE deleted_at IS NULL;

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_mst_employee_level_search
    ON mst_employee_level USING gin(
        to_tsvector('english',
            COALESCE(code, '') || ' ' ||
            COALESCE(name, '')
        )
    ) WHERE deleted_at IS NULL;
