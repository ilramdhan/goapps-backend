-- IAM Service Database Migrations
-- 000030: Create mst_company_mapping + user_company_mappings junction.
--
-- A "company mapping" is a denormalized organizational path
-- (Company → Division → Department → optional Section) bundled under a
-- single human-friendly code. Users can be assigned one or more mappings,
-- with at most one marked as primary.

-- =============================================================================
-- COMPANY MAPPING TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_company_mapping (
    company_mapping_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    company_id UUID NOT NULL,
    division_id UUID NOT NULL,
    department_id UUID NOT NULL,
    section_id UUID,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT uq_company_mapping_code UNIQUE (code),
    CONSTRAINT chk_company_mapping_code_format CHECK (code ~ '^[A-Z][A-Z0-9-]*$'),
    CONSTRAINT fk_company_mapping_company    FOREIGN KEY (company_id)    REFERENCES mst_company(company_id)       ON DELETE RESTRICT,
    CONSTRAINT fk_company_mapping_division   FOREIGN KEY (division_id)   REFERENCES mst_division(division_id)     ON DELETE RESTRICT,
    CONSTRAINT fk_company_mapping_department FOREIGN KEY (department_id) REFERENCES mst_department(department_id) ON DELETE RESTRICT,
    CONSTRAINT fk_company_mapping_section    FOREIGN KEY (section_id)    REFERENCES mst_section(section_id)       ON DELETE RESTRICT
);

-- A mapping is conceptually unique by its hierarchy combo (section nullable counts).
CREATE UNIQUE INDEX IF NOT EXISTS unique_mapping_combo
    ON mst_company_mapping (
        company_id,
        division_id,
        department_id,
        COALESCE(section_id, '00000000-0000-0000-0000-000000000000'::uuid)
    )
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_company_mapping_code       ON mst_company_mapping(code)          WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_company_mapping_active     ON mst_company_mapping(is_active)     WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_company_mapping_company    ON mst_company_mapping(company_id)    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_company_mapping_division   ON mst_company_mapping(division_id)   WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_company_mapping_department ON mst_company_mapping(department_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_company_mapping_section    ON mst_company_mapping(section_id)    WHERE deleted_at IS NULL;

COMMENT ON TABLE  mst_company_mapping IS 'Denormalized organizational mapping (Company→Division→Department→Section).';
COMMENT ON COLUMN mst_company_mapping.code IS 'Unique uppercase code, e.g. MGT-SLO-FIN.';

-- =============================================================================
-- USER ↔ COMPANY MAPPING JUNCTION
-- =============================================================================
CREATE TABLE IF NOT EXISTS user_company_mappings (
    user_id            UUID NOT NULL,
    company_mapping_id UUID NOT NULL,
    is_primary         BOOLEAN NOT NULL DEFAULT false,
    assigned_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    assigned_by        VARCHAR(100) NOT NULL,
    PRIMARY KEY (user_id, company_mapping_id),
    CONSTRAINT fk_ucm_user    FOREIGN KEY (user_id)            REFERENCES mst_user(user_id)                              ON DELETE CASCADE,
    CONSTRAINT fk_ucm_mapping FOREIGN KEY (company_mapping_id) REFERENCES mst_company_mapping(company_mapping_id)        ON DELETE RESTRICT
);

-- At most one primary mapping per user.
CREATE UNIQUE INDEX IF NOT EXISTS unique_user_primary_mapping
    ON user_company_mappings(user_id)
    WHERE is_primary = true;

CREATE INDEX IF NOT EXISTS idx_ucm_mapping ON user_company_mappings(company_mapping_id);

COMMENT ON TABLE user_company_mappings IS 'User-to-company-mapping junction. At most one row per user has is_primary=true.';
