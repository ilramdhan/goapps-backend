-- Down: recreate legacy mst_product_type with copied rows then drop cost_product_type.
CREATE TABLE IF NOT EXISTS mst_product_type (
    product_type_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type_code       VARCHAR(10) NOT NULL,
    type_name       VARCHAR(100) NOT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ
);
INSERT INTO mst_product_type (type_code, type_name, is_active, created_at, updated_at)
SELECT cpt_type_code, cpt_type_name, cpt_is_active, cpt_created_at, cpt_updated_at
FROM cost_product_type
ON CONFLICT DO NOTHING;
DROP TABLE IF EXISTS cost_product_type;
