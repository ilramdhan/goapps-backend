-- Create mst_parameter table for Parameter master data.
CREATE TABLE IF NOT EXISTS mst_parameter (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    param_code VARCHAR(20) NOT NULL,
    param_name VARCHAR(200) NOT NULL,
    param_short_name VARCHAR(50) DEFAULT '',
    data_type VARCHAR(20) NOT NULL CHECK (data_type IN ('NUMBER', 'TEXT', 'BOOLEAN')),
    param_category VARCHAR(20) NOT NULL CHECK (param_category IN ('INPUT', 'RATE', 'CALCULATED')),
    uom_id UUID REFERENCES mst_uom(uom_id),
    default_value DECIMAL(20,6),
    min_value DECIMAL(20,6),
    max_value DECIMAL(20,6),
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    created_by VARCHAR(200) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(200),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(200)
);

-- Unique constraint on param_code (only non-deleted)
CREATE UNIQUE INDEX IF NOT EXISTS idx_mst_parameter_code
    ON mst_parameter (param_code)
    WHERE deleted_at IS NULL;

-- Index for active records
CREATE INDEX IF NOT EXISTS idx_mst_parameter_active
    ON mst_parameter (is_active)
    WHERE deleted_at IS NULL;

-- Index for data_type filter
CREATE INDEX IF NOT EXISTS idx_mst_parameter_data_type
    ON mst_parameter (data_type)
    WHERE deleted_at IS NULL;

-- Index for param_category filter
CREATE INDEX IF NOT EXISTS idx_mst_parameter_category
    ON mst_parameter (param_category)
    WHERE deleted_at IS NULL;

-- Index for UOM FK
CREATE INDEX IF NOT EXISTS idx_mst_parameter_uom_id
    ON mst_parameter (uom_id)
    WHERE uom_id IS NOT NULL AND deleted_at IS NULL;

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_mst_parameter_search
    ON mst_parameter USING gin(to_tsvector('english', coalesce(param_code, '') || ' ' || coalesce(param_name, '') || ' ' || coalesce(param_short_name, '')))
    WHERE deleted_at IS NULL;
