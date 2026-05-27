-- LOOKUP masters that replace hardcoded proto enums in cost_product_request.
-- Step 1 of S7.13 scope. Frontend can now manage raw material types and box
-- types via standard CRUD instead of editing proto files for new options.
--
-- The existing CHECK constraint on cost_product_spec (chk_cps_raw_mat / chk_cps_box_type)
-- still enforces the original enum strings, so seed values mirror the proto enum
-- one-for-one. New rows added later won't be usable in cost_product_spec until
-- the CHECK constraints are relaxed in a follow-up migration.

CREATE TABLE IF NOT EXISTS mst_raw_material_type (
    raw_material_type_id  SERIAL PRIMARY KEY,
    type_code             VARCHAR(30) UNIQUE NOT NULL,
    type_name             VARCHAR(100) NOT NULL,
    description           TEXT,
    is_active             BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by            VARCHAR(100) NOT NULL,
    updated_at            TIMESTAMPTZ,
    updated_by            VARCHAR(100),
    deleted_at            TIMESTAMPTZ,
    deleted_by            VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_mst_raw_material_type_active
    ON mst_raw_material_type(type_code)
    WHERE deleted_at IS NULL AND is_active = TRUE;

INSERT INTO mst_raw_material_type (type_code, type_name, description, created_by) VALUES
    ('POY_BOUGHTOUT', 'POY (bought-out)',      'Partially Oriented Yarn sourced externally', 'seed'),
    ('CHIPS_SD',      'Polymer chips — SD',    'Semi-Dull polyester chips, primary spinning feedstock', 'seed'),
    ('CHIPS_BRT',     'Polymer chips — BRT',   'Bright polyester chips', 'seed'),
    ('CHIPS_RECYCLE', 'Polymer chips — recycle','Recycled polyester chips', 'seed')
ON CONFLICT (type_code) DO NOTHING;

CREATE TABLE IF NOT EXISTS mst_box_type (
    box_type_id   SERIAL PRIMARY KEY,
    type_code     VARCHAR(30) UNIQUE NOT NULL,
    type_name     VARCHAR(100) NOT NULL,
    description   TEXT,
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by    VARCHAR(100) NOT NULL,
    updated_at    TIMESTAMPTZ,
    updated_by    VARCHAR(100),
    deleted_at    TIMESTAMPTZ,
    deleted_by    VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_mst_box_type_active
    ON mst_box_type(type_code)
    WHERE deleted_at IS NULL AND is_active = TRUE;

INSERT INTO mst_box_type (type_code, type_name, description, created_by) VALUES
    ('JUMBO',  'Jumbo box',  'Large carton, FG export packing', 'seed'),
    ('NORMAL', 'Normal box', 'Standard carton', 'seed'),
    ('PALLET', 'Pallet',     'Wooden/plastic pallet only, no carton', 'seed')
ON CONFLICT (type_code) DO NOTHING;
