-- 000494: cpp_value_mb_spin_id on cost_product_parameter — permanent-identity
-- companion column for MB_SPIN-lookup parameters.
--
-- CONTEXT (U-mbspin-lookup-detail, putaran ke-3, Bagian 2): cost_product_parameter
-- rows whose mst_parameter.data_type = TEXT and whose lookup master is MB_SPIN
-- store the raw string the user picked in cpp_value_text (ORION item code, or
-- since the Bagian 1 fix, the mst_mb_spin.mbs_id string when the spin has no
-- ORION code). Production data shows the ORION code is NOT unique: 177 codes are
-- shared by more than one mst_mb_spin row (max 16 rows per code, average 2.63
-- rows per duplicated code, 466 rows involved). A code string alone cannot be
-- trusted as a single-row identity.
--
-- DESIGN — companion, NOT a replacement (final, non-negotiable per orchestrator):
--   * cpp_value_text keeps being populated exactly as before. cpp_one_value_chk
--     is untouched and is satisfied the same way it always was.
--   * cpp_value_mb_spin_id is an ADDITIONAL, independent pointer straight at
--     mst_mb_spin.mbs_id (the permanent primary key), populated by the backend
--     ONLY when the incoming value resolves unambiguously to exactly one
--     mst_mb_spin row. Ambiguous (duplicate-code) resolutions leave this column
--     NULL rather than guessing a row — see the application-layer resolver.
--
-- NO BACKFILL. This migration is pure DDL: new column + index + FK + comment.
-- Every existing row's cpp_value_mb_spin_id stays NULL forever — there is no
-- reliable way to retroactively resolve old cpp_value_text strings to a single
-- mst_mb_spin row (that's the same ambiguity problem this column exists to avoid
-- for new writes), so guessing an owner for old data would be worse than leaving
-- it unset. NULL here means "not yet linked to a permanent id", not an error.
--
-- Style follows the nearest neighbor migrations (000490, 000493): idempotent
-- ADD COLUMN IF NOT EXISTS / DO-block guarded ADD CONSTRAINT / IF NOT EXISTS
-- indexes, explicit BEGIN/COMMIT, COMMENT ON COLUMN documenting the hybrid
-- schema and the permanent-NULL-for-old-rows rule.

BEGIN;

-- Column. No NOT NULL, no DEFAULT — this ALTER TABLE is a catalog-only change
-- and rewrites no rows.
ALTER TABLE cost_product_parameter
    ADD COLUMN IF NOT EXISTS cpp_value_mb_spin_id UUID;

-- FK, idempotent via DO-block (ADD CONSTRAINT has no IF NOT EXISTS in
-- PostgreSQL), mirroring 000490's fk_mbs_cost_product pattern.
-- ON DELETE SET NULL: this pointer is an identity aid for the MB_SPIN lookup,
-- not a business fact that should block deleting the referenced mst_mb_spin row.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_cpp_value_mb_spin'
    ) THEN
        ALTER TABLE cost_product_parameter
            ADD CONSTRAINT fk_cpp_value_mb_spin
                FOREIGN KEY (cpp_value_mb_spin_id)
                REFERENCES mst_mb_spin (mbs_id)
                ON DELETE SET NULL;
    END IF;
END
$$;

-- Partial, non-unique index, mirroring idx_mbs_cost_product_id (000490):
--   * PARTIAL (WHERE ... IS NOT NULL): every pre-existing row and every
--     ambiguous-resolution row is NULL by design; no query searches for NULL
--     through this index.
--   * NON-UNIQUE: nothing prevents (and nothing should prevent) more than one
--     cost_product_parameter row from pointing at the same mst_mb_spin row.
CREATE INDEX IF NOT EXISTS idx_cpp_value_mb_spin_id
    ON cost_product_parameter (cpp_value_mb_spin_id)
    WHERE cpp_value_mb_spin_id IS NOT NULL;

COMMENT ON COLUMN cost_product_parameter.cpp_value_mb_spin_id IS
    'Permanent-identity companion to cpp_value_text for MB_SPIN-lookup parameters. '
    'Points at mst_mb_spin.mbs_id when the backend could resolve the saved value to '
    'exactly one spin row; stays NULL when the value was ambiguous (duplicate ORION '
    'code, matches >1 row) or when the row predates this column (NO BACKFILL was '
    'run — see 000494 up migration). NULL never invalidates cpp_one_value_chk, which '
    'is unchanged and still evaluated against cpp_value_numeric/cpp_value_text/'
    'cpp_value_flag only. Application code must never treat NULL here as an error; '
    'it must fall back to resolving via cpp_value_text as before this column existed.';

COMMIT;
