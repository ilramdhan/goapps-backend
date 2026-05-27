-- Create Employee Group master table
CREATE TABLE IF NOT EXISTS mst_employee_group (
    employee_group_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Core fields
    code VARCHAR(20) NOT NULL,
    name VARCHAR(100) NOT NULL,

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
CREATE UNIQUE INDEX IF NOT EXISTS idx_mst_employee_group_code
    ON mst_employee_group(code) WHERE deleted_at IS NULL;

-- Index for active filter
CREATE INDEX IF NOT EXISTS idx_mst_employee_group_active
    ON mst_employee_group(is_active) WHERE deleted_at IS NULL;

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_mst_employee_group_search
    ON mst_employee_group USING gin(
        to_tsvector('english',
            COALESCE(code, '') || ' ' ||
            COALESCE(name, '')
        )
    ) WHERE deleted_at IS NULL;
