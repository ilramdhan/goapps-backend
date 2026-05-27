-- Migration: defensively re-assert width on cst_rm_group_detail text columns.
--
-- Context: operators hit a "value too long for type character varying(30)"
-- (SQLSTATE 22001) error when adding items to a freshly re-created group.
-- Migration 000015 already widened item_name, item_type_code and item_grade,
-- but this re-asserts the widening idempotently so the error cannot reappear
-- on databases where 000015 got stuck as dirty or was never applied.
-- ALTER TABLE ... TYPE to a wider VARCHAR is a no-op when the column is
-- already at least that wide, so this migration is safe to run on any DB.
--
-- Also widens grade_code to 60 (source is 40) to give margin for future data,
-- and widens *_by audit columns to 150 so longer emails fit.

ALTER TABLE cst_rm_group_detail
    ALTER COLUMN item_name       TYPE VARCHAR(240),
    ALTER COLUMN item_type_code  TYPE VARCHAR(60),
    ALTER COLUMN item_grade      TYPE VARCHAR(240),
    ALTER COLUMN grade_code      TYPE VARCHAR(60),
    ALTER COLUMN created_by      TYPE VARCHAR(150),
    ALTER COLUMN updated_by      TYPE VARCHAR(150),
    ALTER COLUMN deleted_by      TYPE VARCHAR(150);

-- aud_rm_group_detail: match detail widenings so audit rows do not overflow
-- when the detail row fits.
ALTER TABLE aud_rm_group_detail
    ALTER COLUMN grade_code   TYPE VARCHAR(60),
    ALTER COLUMN changed_by   TYPE VARCHAR(150);
