-- 000525 down — remove only the CAPP rows seeded by 000525 (marker).
-- 000526's down (run first in reverse order) has already removed the OIL_NAME
-- cost_product_parameter rows it inserted for these products.

BEGIN;

DELETE FROM cost_product_applicable_param
WHERE capp_created_by = 'seed_oil_capp_000525';

COMMIT;
