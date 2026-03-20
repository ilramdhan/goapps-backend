-- IAM Service Database Migrations
-- 000002: Create user tables
--
-- Tables: mst_user, mst_user_detail
-- User credentials and employee profile information

-- =============================================================================
-- USER TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_user (
    user_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(100) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_locked BOOLEAN NOT NULL DEFAULT false,
    failed_login_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMP WITH TIME ZONE,
    two_factor_enabled BOOLEAN NOT NULL DEFAULT false,
    two_factor_secret VARCHAR(100),  -- Encrypted TOTP secret
    last_login_at TIMESTAMP WITH TIME ZONE,
    last_login_ip VARCHAR(50),
    password_changed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMP WITH TIME ZONE,
    deleted_by VARCHAR(100),
    CONSTRAINT chk_username_format CHECK (username ~ '^[a-zA-Z][a-zA-Z0-9_]{2,49}$'),
    CONSTRAINT chk_email_format CHECK (email ~ '^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$')
);

-- Partial unique indexes: allow re-creating soft-deleted users with same username/email
CREATE UNIQUE INDEX uq_user_username ON mst_user(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_user_email ON mst_user(email) WHERE deleted_at IS NULL;

-- User indexes
CREATE INDEX idx_user_username ON mst_user(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_email ON mst_user(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_active ON mst_user(is_active) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_locked ON mst_user(is_locked) WHERE deleted_at IS NULL;
CREATE INDEX idx_user_search ON mst_user USING gin(
    (username || ' ' || email) gin_trgm_ops
) WHERE deleted_at IS NULL;

COMMENT ON TABLE mst_user IS 'Master table for user credentials and authentication';
COMMENT ON COLUMN mst_user.password_hash IS 'Argon2id hashed password';
COMMENT ON COLUMN mst_user.two_factor_secret IS 'TOTP secret for 2FA, encrypted at rest';

-- =============================================================================
-- USER DETAIL TABLE
-- =============================================================================
CREATE TABLE IF NOT EXISTS mst_user_detail (
    detail_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    section_id UUID,  -- Optional, employee may not belong to a section
    employee_code VARCHAR(20) NOT NULL,
    full_name VARCHAR(100) NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    phone VARCHAR(20),
    profile_picture_url TEXT,
    position VARCHAR(50),
    date_of_birth DATE,
    address TEXT,
    extra_data JSONB,  -- Extensible fields
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE,
    updated_by VARCHAR(100),
    CONSTRAINT fk_user_detail_user FOREIGN KEY (user_id) REFERENCES mst_user(user_id) ON DELETE CASCADE,
    CONSTRAINT fk_user_detail_section FOREIGN KEY (section_id) REFERENCES mst_section(section_id) ON DELETE SET NULL,
    CONSTRAINT uq_user_detail_user UNIQUE (user_id),
    CONSTRAINT uq_user_detail_employee_code UNIQUE (employee_code)
);

-- User detail indexes
CREATE INDEX idx_user_detail_user ON mst_user_detail(user_id);
CREATE INDEX idx_user_detail_section ON mst_user_detail(section_id);
CREATE INDEX idx_user_detail_employee_code ON mst_user_detail(employee_code);
CREATE INDEX idx_user_detail_search ON mst_user_detail USING gin(
    (employee_code || ' ' || full_name) gin_trgm_ops
);

COMMENT ON TABLE mst_user_detail IS 'User profile and employee information';
COMMENT ON COLUMN mst_user_detail.extra_data IS 'Extensible JSON fields for custom employee attributes';
