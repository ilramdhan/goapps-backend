-- 000495: PARTIAL backfill of cost_product_parameter.cpp_value_mb_spin_id
-- (companion column added by 000494, NO BACKFILL at the time) for legacy
-- MB_SPIN rows only.
--
-- SCOPE — MB_SPIN parameter only. Joins mst_parameter via
-- param_code = 'MB_SP_CODE' AND deleted_at IS NULL (that param_code has
-- exactly one non-deleted row in production; mst_parameter.id is NOT
-- hardcoded here on purpose — it is gen_random_uuid()-generated, so any
-- join MUST go through param_code, never a literal UUID).
--
-- WHY PARTIAL, NOT FULL: cpp_value_text for this parameter historically
-- stored the ORION item code (mst_mb_spin.mbs_orion_item_code), and that
-- code is NOT unique in mst_mb_spin — 177+ codes are shared by more than
-- one active spin row. A code string alone cannot always be trusted to
-- identify a single mst_mb_spin row, so this migration only fills the
-- column where the code resolves to EXACTLY ONE non-deleted mst_mb_spin
-- row. Rows whose code matches zero or more than one active spin are
-- intentionally left NULL — NULL here means "not yet known which spin
-- this is", it is NOT an error and NOT a data-quality flag.
--
-- Matching semantics deliberately mirror the application save path
-- (mb_spin_repository.go:243-268 as of 2026-08-27): exact equality (=) on
-- mbs_orion_item_code against non-deleted (deleted_at IS NULL) mst_mb_spin
-- rows only. No TRIM(), no UPPER()/LOWER(), no fuzzy matching — this
-- migration must reproduce the exact same ambiguity rules the app uses at
-- write time, not a looser or stricter one, so that a value this migration
-- fills is a value the application itself would also have resolved
-- unambiguously had the row been saved today.
--
-- PRODUCTION MEASUREMENT (2026-08-27 preflight against real data, see
-- orchestrator report — cited here for context only, not re-verified by
-- this migration):
--   * MB_SPIN legacy cost_product_parameter rows (cpp_value_mb_spin_id
--     IS NULL, cpp_value_text IS NOT NULL, param_code = MB_SP_CODE): 2256
--   * Rows with a code but empty/missing text: 0
--   * Resolvable to EXACTLY ONE active mst_mb_spin row -> WILL be filled
--     by this migration: 1208 (53.5%)
--   * Ambiguous (code matches >1 active mst_mb_spin row) -> intentionally
--     LEFT NULL by this migration: 1048
--   * Orphan (code matches 0 active mst_mb_spin row, e.g. typo/case/space
--     mismatch): 0 — confirmed no whitespace/casing issues exist, which is
--     also exactly why this migration does not TRIM/UPPER anything: there
--     is nothing in production for that leniency to fix, and adding it
--     anyway would silently change matching semantics away from the
--     application's own exact-match rule for no measured benefit.
--
-- ⛔ SAFETY RULES THIS MIGRATION MUST NOT VIOLATE (non-negotiable):
--   * Ambiguous rows (>1 match) MUST be skipped, left NULL. This migration
--     deliberately does NOT use LIMIT 1 / ORDER BY ... LIMIT 1 /
--     DISTINCT ON / MIN(mbs_id) anywhere — every one of those would be a
--     GUESS among multiple candidate spins, and a wrong guess would
--     silently lock a product to the wrong MB Spin and feed a wrong number
--     into cost calculations that read this column. Ambiguity is resolved
--     by requiring an exact COUNT(*) = 1 in the WHERE clause instead (see
--     below), so the SET subquery only ever executes where uniqueness is
--     already proven.
--   * cpp_value_text is NEVER written by this migration — it is read-only
--     input here, exactly as it was before 000494/000495 existed.
--   * Only cpp_value_mb_spin_id IS NULL rows are touched — rows already
--     resolved by the live application save path (or by a previous run of
--     this same migration) are left completely alone, both for those
--     rows' cpp_value_mb_spin_id and for cpp_updated_at/cpp_updated_by
--     (see audit-column note below).
--   * IDEMPOTENT: the WHERE cpp.cpp_value_mb_spin_id IS NULL guard means a
--     second run of this migration finds zero eligible rows (every row it
--     could have touched the first time is no longer NULL) and performs a
--     0-row UPDATE. Re-running is safe and a no-op.
--
-- AUDIT COLUMNS — DECISION GATE, reported rather than decided silently:
-- cost_product_parameter also has cpp_updated_at/cpp_updated_by
-- (000217_create_cost_product_parameter.up.sql). This migration
-- deliberately does NOT set either of them for the rows it backfills.
-- Setting them (e.g. cpp_updated_by = 'migration_000495') would let a
-- future down-migration or audit query tell backfilled rows apart from
-- rows the live save path resolved -- but cpp_updated_at/cpp_updated_by
-- are also the "who/when last touched this row" trail that other features
-- may surface (e.g. showing the last human editor of a cost parameter
-- value). Stamping them from this migration would make a bulk backfill
-- masquerade as a human edit event, which is a worse trade-off than losing
-- the ability to distinguish backfill-origin rows later. See the .down.sql
-- of this migration for the direct consequence of this choice: it CANNOT
-- safely revert only the rows this UP migration touched, and says so
-- rather than guessing.
--
-- Style follows 000490/000493/000494: explicit BEGIN/COMMIT, comments
-- ahead of the statement they explain.

BEGIN;

UPDATE cost_product_parameter cpp
   SET cpp_value_mb_spin_id = (
        SELECT mbs.mbs_id
          FROM mst_mb_spin mbs
         WHERE mbs.mbs_orion_item_code = cpp.cpp_value_text
           AND mbs.deleted_at IS NULL
       )
  FROM mst_parameter mp
 WHERE cpp.cpp_param_id = mp.id
   AND mp.param_code = 'MB_SP_CODE'
   AND mp.deleted_at IS NULL
   AND cpp.cpp_value_mb_spin_id IS NULL
   AND cpp.cpp_value_text IS NOT NULL
   -- Uniqueness gate: the SET subquery above is only ever reached for rows
   -- where this COUNT is proven to be exactly 1, so it can never return
   -- more than one row (no "more than one row returned by a subquery"
   -- error) and never silently picks among candidates.
   AND (
        SELECT COUNT(*)
          FROM mst_mb_spin mbs
         WHERE mbs.mbs_orion_item_code = cpp.cpp_value_text
           AND mbs.deleted_at IS NULL
       ) = 1;

COMMIT;
