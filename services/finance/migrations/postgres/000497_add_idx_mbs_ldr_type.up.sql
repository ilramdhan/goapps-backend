-- 000497: supporting index for mst_mb_spin.mbs_ldr_type (added by 000496).
--
-- Research note (per task instructions): checked for any other pending need
-- (e.g. a mst_formula/lookup seed row for the new LDR-type states) before
-- reserving this migration number. Found none — mbs_ldr_type is a plain
-- VARCHAR + CHECK tri-state, not backed by any lookup/master table (unlike
-- e.g. mst_mb_cross_section for mbh_cross_section), so there is nothing to
-- seed. This migration is therefore exactly what the task fallback describes:
-- a small, standalone index addition, not invented schema.
--
-- Purpose: the planned recalculation cascade will need to find, per MB Head,
-- which child MB Spin rows are eligible for recompute (mbs_ldr_type IN
-- ('NOT_CALCULATED','CALCULATED') — i.e. anything NOT locked as ACTUAL) among
-- live (non-soft-deleted) rows. This index supports that filter.
--
-- PARTIAL, non-unique, mirroring the partial-index precedent already used on
-- this table (idx_mst_mb_spin_mbh_id / idx_mst_mb_spin_mgt_name, 000389):
-- excluding soft-deleted rows keeps the index small and matches how those
-- existing indexes already narrow to the live subset of mst_mb_spin.
--
-- CREATE INDEX CONCURRENTLY is not used, following the same house
-- constraint documented in 000490: golang-migrate here runs each migration
-- file as a single implicit transaction block, and PostgreSQL rejects
-- CONCURRENTLY inside a transaction. A brief ACCESS EXCLUSIVE lock on
-- mst_mb_spin is expected; run in a low-traffic window like other plain
-- CREATE INDEX migrations in this directory.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_mbs_ldr_type
    ON mst_mb_spin (mbs_ldr_type)
    WHERE deleted_at IS NULL;

COMMIT;
