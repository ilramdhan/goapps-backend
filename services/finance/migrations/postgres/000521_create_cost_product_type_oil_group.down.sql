-- 000521 down — drop the product type -> oil group mapping and cpt_oil_class.

BEGIN;

DROP TABLE IF EXISTS cost_product_type_oil_group;

ALTER TABLE cost_product_type
    DROP CONSTRAINT IF EXISTS chk_cpt_oil_class;

ALTER TABLE cost_product_type
    DROP COLUMN IF EXISTS cpt_oil_class;

COMMIT;
