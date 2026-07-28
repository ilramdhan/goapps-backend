-- Machine group master. PPC-owned (no finance equivalent). Groups machines by
-- area for capacity planning and threshold resolution.
CREATE TABLE IF NOT EXISTS machine_group (
    group_id    BIGSERIAL    PRIMARY KEY,
    group_name  VARCHAR(50)  NOT NULL,
    group_area  CHAR(3)      NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by  VARCHAR(100) NOT NULL DEFAULT 'system',
    updated_at  TIMESTAMPTZ,
    updated_by  VARCHAR(100),
    CONSTRAINT chk_machine_group_area CHECK (group_area IN ('TXT', 'SPG', 'TWT')),
    CONSTRAINT uq_machine_group_name_area UNIQUE (group_name, group_area)
);

CREATE INDEX IF NOT EXISTS idx_machine_group_area ON machine_group (group_area);
