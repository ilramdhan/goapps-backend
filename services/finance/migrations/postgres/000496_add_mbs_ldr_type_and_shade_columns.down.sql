-- Down 000496 — drop exactly what the up migration added, nothing more.
--
-- DROP COLUMN on Postgres automatically removes any CHECK constraint
-- defined on that column (chk_mbs_ldr_type goes away with mbs_ldr_type), so
-- no separate DROP CONSTRAINT statement is needed — this mirrors how
-- 000490's down migration still drops mbs_cost_product_id's FK explicitly
-- for clarity, but here the constraint is a plain column CHECK, not a named
-- FK other code might reference, so relying on the implicit drop is safe
-- and matches PostgreSQL's own documented behavior.
--
-- ⛔ DESTRUCTIVE for mbs_ldr_calculated_pct / mbs_ldr_adjustment_pct /
-- mbs_ldr_type / mbs_ldr_is_actual / mbs_shade_code / mbs_shade_name /
-- mbs_cross_section: any values written into these columns after 000496
-- ran are permanently lost. Only run this if the LDR-type feature is truly
-- being rolled back.
--
-- ⛔ Does NOT touch anything owned by other migrations: mst_mb_head's
-- mbh_shade_code/mbh_shade_name/mbh_cross_section (000445), the pre-existing
-- mbs_ldr_is_fixed/mbs_dozing_is_fixed (000486), or mbs_ldr_prsn/
-- mbs_run_ldr_pct (000414/000418) are all left untouched.

BEGIN;

ALTER TABLE mst_mb_spin
    DROP COLUMN IF EXISTS mbs_ldr_is_actual,
    DROP COLUMN IF EXISTS mbs_ldr_adjustment_pct,
    DROP COLUMN IF EXISTS mbs_ldr_calculated_pct,
    DROP COLUMN IF EXISTS mbs_ldr_type,
    DROP COLUMN IF EXISTS mbs_cross_section,
    DROP COLUMN IF EXISTS mbs_shade_name,
    DROP COLUMN IF EXISTS mbs_shade_code;

COMMIT;
