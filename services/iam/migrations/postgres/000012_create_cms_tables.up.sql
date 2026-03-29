-- CMS Pages: full-page content (Privacy Policy, Terms of Service, etc.)
CREATE TABLE IF NOT EXISTS mst_cms_page (
    page_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    page_slug VARCHAR(100) NOT NULL,
    page_title VARCHAR(200) NOT NULL,
    page_content TEXT NOT NULL DEFAULT '',
    meta_description VARCHAR(500) DEFAULT '',
    is_published BOOLEAN NOT NULL DEFAULT false,
    published_at TIMESTAMPTZ,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMPTZ,
    deleted_by VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cms_page_slug
    ON mst_cms_page(page_slug) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_cms_page_published
    ON mst_cms_page(is_published) WHERE deleted_at IS NULL;

-- CMS Sections: landing page sections (hero, features, FAQ, etc.)
CREATE TABLE IF NOT EXISTS mst_cms_section (
    section_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    section_type VARCHAR(50) NOT NULL DEFAULT 'CUSTOM',
    section_key VARCHAR(100) NOT NULL,
    title VARCHAR(200) NOT NULL DEFAULT '',
    subtitle VARCHAR(500) DEFAULT '',
    content TEXT DEFAULT '',
    icon_name VARCHAR(100) DEFAULT '',
    image_url VARCHAR(500) DEFAULT '',
    button_text VARCHAR(100) DEFAULT '',
    button_url VARCHAR(500) DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    is_published BOOLEAN NOT NULL DEFAULT false,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),
    deleted_at TIMESTAMPTZ,
    deleted_by VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cms_section_key
    ON mst_cms_section(section_key) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_cms_section_type
    ON mst_cms_section(section_type) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_cms_section_published
    ON mst_cms_section(is_published, sort_order) WHERE deleted_at IS NULL;

-- CMS Settings: key-value site configuration (no soft delete)
CREATE TABLE IF NOT EXISTS mst_cms_setting (
    setting_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    setting_key VARCHAR(100) NOT NULL UNIQUE,
    setting_value TEXT NOT NULL DEFAULT '',
    setting_type VARCHAR(50) NOT NULL DEFAULT 'TEXT',
    setting_group VARCHAR(100) NOT NULL DEFAULT 'general',
    description VARCHAR(500) DEFAULT '',
    is_editable BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_cms_setting_group
    ON mst_cms_setting(setting_group);
