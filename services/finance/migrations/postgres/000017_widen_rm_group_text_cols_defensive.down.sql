-- Revert defensive widenings applied in 000017. Narrowing a VARCHAR can fail
-- if existing rows exceed the target width; callers must clean up before
-- rolling back.

ALTER TABLE aud_rm_group_detail
    ALTER COLUMN grade_code   TYPE VARCHAR(40),
    ALTER COLUMN changed_by   TYPE VARCHAR(100);

ALTER TABLE cst_rm_group_detail
    ALTER COLUMN item_name       TYPE VARCHAR(200),
    ALTER COLUMN item_type_code  TYPE VARCHAR(30),
    ALTER COLUMN item_grade      TYPE VARCHAR(30),
    ALTER COLUMN grade_code      TYPE VARCHAR(40),
    ALTER COLUMN created_by      TYPE VARCHAR(100),
    ALTER COLUMN updated_by      TYPE VARCHAR(100),
    ALTER COLUMN deleted_by      TYPE VARCHAR(100);
