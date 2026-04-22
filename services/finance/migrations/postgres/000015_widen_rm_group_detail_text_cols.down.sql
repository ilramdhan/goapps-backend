-- Rollback: restore the original widths. This may fail if rows already contain
-- values longer than the old limits; in that case truncate or clean up first.

ALTER TABLE cst_rm_group_detail
    ALTER COLUMN item_name       TYPE VARCHAR(200),
    ALTER COLUMN item_type_code  TYPE VARCHAR(30),
    ALTER COLUMN item_grade      TYPE VARCHAR(30);
