-- Machine master. SYNC-SOURCED (anti-drift, gap R1/§5.2): rows are upserted from
-- finance mst_machine (via gRPC) + Oracle TXTMACH, never hand-authored. PPC-only
-- fields (area/group/doff) are locally editable and preserved across syncs.
CREATE TABLE IF NOT EXISTS machine (
    machine_id             BIGSERIAL    PRIMARY KEY,
    machine_no             VARCHAR(10)  NOT NULL,
    machine_area           CHAR(3)      NOT NULL,
    machine_line           VARCHAR(20),
    machine_group_id       BIGINT,
    machine_doff_weight_kg DECIMAL(8,3),
    machine_is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    machine_orion_code     VARCHAR(30),
    source_mc_id           UUID,        -- finance mst_machine.mc_id (nullable when Oracle-only)
    synced_at              TIMESTAMPTZ, -- last sync timestamp
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by             VARCHAR(100) NOT NULL DEFAULT 'system',
    updated_at             TIMESTAMPTZ,
    updated_by             VARCHAR(100),
    CONSTRAINT chk_machine_area CHECK (machine_area IN ('TXT', 'SPG', 'TWT')),
    CONSTRAINT uq_machine_no UNIQUE (machine_no)
);

CREATE INDEX IF NOT EXISTS idx_machine_area ON machine (machine_area);
CREATE INDEX IF NOT EXISTS idx_machine_group_id ON machine (machine_group_id);
