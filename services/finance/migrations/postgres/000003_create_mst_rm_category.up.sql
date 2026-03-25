-- Create Raw Material Category master table
CREATE TABLE IF NOT EXISTS mst_rm_category (
    rm_category_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Core fields
    category_code VARCHAR(20) NOT NULL,
    category_name VARCHAR(100) NOT NULL,
    description TEXT,

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
CREATE UNIQUE INDEX IF NOT EXISTS idx_mst_rm_category_code
    ON mst_rm_category(category_code) WHERE deleted_at IS NULL;

-- Index for active filter
CREATE INDEX IF NOT EXISTS idx_mst_rm_category_active
    ON mst_rm_category(is_active) WHERE deleted_at IS NULL;

-- Index for created_at sorting
CREATE INDEX IF NOT EXISTS idx_mst_rm_category_created_at
    ON mst_rm_category(created_at) WHERE deleted_at IS NULL;

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_mst_rm_category_search
    ON mst_rm_category USING gin(
        to_tsvector('english',
            COALESCE(category_code, '') || ' ' ||
            COALESCE(category_name, '') || ' ' ||
            COALESCE(description, '')
        )
    ) WHERE deleted_at IS NULL;
