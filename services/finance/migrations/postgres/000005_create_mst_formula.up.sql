-- Create mst_formula table and formula_param junction table.
CREATE TABLE IF NOT EXISTS mst_formula (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    formula_code VARCHAR(50) NOT NULL,
    formula_name VARCHAR(200) NOT NULL,
    formula_type VARCHAR(20) NOT NULL CHECK (formula_type IN ('CALCULATION', 'SQL_QUERY', 'CONSTANT')),
    expression TEXT NOT NULL,
    result_param_id UUID NOT NULL REFERENCES mst_parameter(id),
    description TEXT DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    created_by VARCHAR(200) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(200),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(200)
);

-- Unique constraint on formula_code (only non-deleted)
CREATE UNIQUE INDEX IF NOT EXISTS idx_mst_formula_code
    ON mst_formula (formula_code)
    WHERE deleted_at IS NULL;

-- Index for active records
CREATE INDEX IF NOT EXISTS idx_mst_formula_active
    ON mst_formula (is_active)
    WHERE deleted_at IS NULL;

-- Index for formula_type filter
CREATE INDEX IF NOT EXISTS idx_mst_formula_type
    ON mst_formula (formula_type)
    WHERE deleted_at IS NULL;

-- Index for result_param_id FK
CREATE INDEX IF NOT EXISTS idx_mst_formula_result_param
    ON mst_formula (result_param_id)
    WHERE deleted_at IS NULL;

-- Full-text search index
CREATE INDEX IF NOT EXISTS idx_mst_formula_search
    ON mst_formula USING gin(to_tsvector('english', coalesce(formula_code, '') || ' ' || coalesce(formula_name, '') || ' ' || coalesce(expression, '')))
    WHERE deleted_at IS NULL;

-- Junction table for formula input parameters (no mst_ prefix per convention)
CREATE TABLE IF NOT EXISTS formula_param (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    formula_id UUID NOT NULL REFERENCES mst_formula(id) ON DELETE CASCADE,
    param_id UUID NOT NULL REFERENCES mst_parameter(id),
    sort_order INTEGER NOT NULL DEFAULT 0
);

-- Each parameter can only appear once per formula
CREATE UNIQUE INDEX IF NOT EXISTS idx_formula_param_unique
    ON formula_param (formula_id, param_id);

-- Index for looking up formulas by param
CREATE INDEX IF NOT EXISTS idx_formula_param_param_id
    ON formula_param (param_id);
