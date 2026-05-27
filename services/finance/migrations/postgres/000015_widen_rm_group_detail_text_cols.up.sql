-- Migration: widen text columns on cst_rm_group_detail to match the source
-- cst_item_cons_stk_po (whose grade_name / item_name are VARCHAR(240)). The
-- previous VARCHAR(30) on item_grade triggered 22001 overflow when operators
-- imported items whose Oracle-provided grade description exceeded 30 chars.

ALTER TABLE cst_rm_group_detail
    ALTER COLUMN item_name       TYPE VARCHAR(240),
    ALTER COLUMN item_type_code  TYPE VARCHAR(60),
    ALTER COLUMN item_grade      TYPE VARCHAR(240);
