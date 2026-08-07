-- Backfill CAPP + CPP rows for the 6 Group-C params seeded by 000468.
--
-- WHY THIS EXISTS
-- 000468 created the params and wired their lookup fill-groups correctly, but
-- never checklisted them against any product. The engine gates on that:
--   * LoadCAPP INNER JOINs cost_product_applicable_param -> cost_product_parameter,
--     so a param with no CAPP row AND no CPP value never enters the CAPP map.
--   * loadPerProductFormulas joins mst_formula ON result_param_id = capp_param_id,
--     so a formula whose result param is not checklisted never loads.
-- Result: after the 202604 recalc, all 6 params produced 0 snapshot rows.
-- Migrations are immutable once merged, so the repair lands here rather than in 000468.
--
-- DATA SOURCES (all verified on dev before writing this file)
-- The per-product grade assignment already exists as TEXT on two params seeded
-- long before 000468, each covering exactly the same 13,429 products as MC_NAME:
--   STD_VALUE_LOSS  -> the "NS" grade name  ('Type 4 NS', 'Type 2 NS', ...)  14 distinct
--   VALUE_LOSS      -> the "BC" grade name  ('Type 4 BC', 'Type 4POY BC', ...) 22 distinct
-- 13,429/13,429 of each match mst_product_grade.pg_name exactly — zero orphans —
-- and pg_name is unique among non-deleted grades, so the join cannot fan out.
--
-- Coverage of the payload columns (rows that will actually get a CPP value):
--   NS_LOSS      <- grade.loss_pct           13,314 / 13,429  (115 grades have NULL loss_pct)
--   STD_SP_AX    <- grade.std_selling_price  13,427 / 13,429
--   STD_SP_BC    <- grade.sp_value           13,429 / 13,429
--   TOTAL_FIXED_COST <- machine.mc_tot_fxd_cst 12,831 / 13,429
--                       (40 of 103 machines have no mc_tot_fxd_cst; mc_name is unique)
-- Products whose source column is NULL still get the CAPP row but no CPP value —
-- exactly the shape of a checklisted-but-unfilled param, which the export prints
-- as "-". Seeding a fabricated 0 would be worse: it is indistinguishable from a
-- real zero cost.
--
-- ⚠ CONFIRM-LATER (recorded as C13/C14 in the STATE ledger)
-- (a) The two grade triggers are derived from STD_VALUE_LOSS / VALUE_LOSS rather
--     than being entered independently. If costing ever wants NS/BC grades chosen
--     separately from those legacy params, this backfill becomes a one-time seed
--     and the UI must own the values from then on.
-- (b) 000468's header asserts that 000408 consumes VALUE_LOSS numerically as
--     (1.0 - VALUE_LOSS / 100.0). VALUE_LOSS is data_type TEXT holding grade
--     names, so that expression currently evaluates to a constant 0 in every
--     snapshot (verified: VALUE_LOSS = '0' in all 41,607 ACTUAL snapshots for
--     202604). F_YARN_BC_LOSS_CAP / _DEL / F_YARN_NON_STD_LOSS are therefore
--     silently producing 0 today. That is a PRE-EXISTING defect, NOT introduced
--     here, and fixing it means repointing those three formulas at the numeric
--     NS_LOSS this migration seeds. Deliberately out of scope — it changes
--     computed cost values and needs costing sign-off first.

BEGIN;

-- ============================================================
-- PART 1: Checklist all 6 params (CAPP)
-- ============================================================
-- Scope: the products that already carry the source params, i.e. the yarn cost
-- model set. Not a CROSS JOIN over cost_product_master — bought-out / non-costed
-- products stay untouched, matching the scoping decision made in 000469 PART 4.

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT src.cpp_product_sys_id, np.id, FALSE, NULL::INT, 'backfill_group_c_000470'
FROM cost_product_parameter src
JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
CROSS JOIN mst_parameter np
WHERE sp.param_code IN ('MC_NAME', 'STD_VALUE_LOSS', 'VALUE_LOSS')
  AND np.param_code IN (
      'TOTAL_FIXED_COST', 'NS_LOSS_TYPE', 'BC_LOSS_TYPE',
      'NS_LOSS', 'STD_SP_AX', 'STD_SP_BC'
  )
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = src.cpp_product_sys_id
        AND capp.capp_param_id       = np.id
  );

-- ============================================================
-- PART 2: Grade trigger values (TEXT) — NS_LOSS_TYPE / BC_LOSS_TYPE
-- ============================================================
-- Copied verbatim from the legacy text params. cpp_one_value_chk requires exactly
-- one of numeric/text/flag, so only cpp_value_text is set.

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_filled_by, cpp_created_by
)
SELECT src.cpp_product_sys_id, np.id, src.cpp_value_text,
       'backfill_group_c_000470', 'backfill_group_c_000470'
FROM cost_product_parameter src
JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
JOIN mst_parameter np
     ON np.param_code = CASE sp.param_code
                            WHEN 'STD_VALUE_LOSS' THEN 'NS_LOSS_TYPE'
                            WHEN 'VALUE_LOSS'     THEN 'BC_LOSS_TYPE'
                        END
    AND np.deleted_at IS NULL
WHERE sp.param_code IN ('STD_VALUE_LOSS', 'VALUE_LOSS')
  AND src.cpp_value_text IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = src.cpp_product_sys_id
        AND cpp.cpp_param_id       = np.id
  );

-- ============================================================
-- PART 3: Grade-derived numeric children
-- ============================================================
-- NS_LOSS follows the NS grade (STD_VALUE_LOSS); STD_SP_AX / STD_SP_BC follow the
-- BC grade (VALUE_LOSS) — matching the lookup_fill_group_code wiring in 000468.
-- Rows whose source column is NULL are skipped, leaving the param checklisted but
-- unfilled.

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by
)
SELECT src.cpp_product_sys_id, np.id, m.val,
       'backfill_group_c_000470', 'backfill_group_c_000470'
FROM (VALUES
    ('STD_VALUE_LOSS', 'NS_LOSS',   'loss_pct'),
    ('VALUE_LOSS',     'STD_SP_AX', 'std_selling_price'),
    ('VALUE_LOSS',     'STD_SP_BC', 'sp_value')
) AS map(source_code, target_code, source_col)
JOIN mst_parameter sp ON sp.param_code = map.source_code AND sp.deleted_at IS NULL
JOIN mst_parameter np ON np.param_code = map.target_code AND np.deleted_at IS NULL
JOIN cost_product_parameter src ON src.cpp_param_id = sp.id AND src.cpp_value_text IS NOT NULL
JOIN mst_product_grade g ON g.pg_name = src.cpp_value_text AND g.deleted_at IS NULL
CROSS JOIN LATERAL (
    SELECT CASE map.source_col
               WHEN 'loss_pct'          THEN g.loss_pct
               WHEN 'std_selling_price' THEN g.std_selling_price
               WHEN 'sp_value'          THEN g.sp_value
           END AS val
) m
WHERE m.val IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = src.cpp_product_sys_id
        AND cpp.cpp_param_id       = np.id
  );

-- ============================================================
-- PART 4: TOTAL_FIXED_COST from the machine master
-- ============================================================
-- MC_NAME already holds the machine name as text for all 13,429 products, and
-- mc_name is unique among non-deleted machines.

INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by
)
SELECT src.cpp_product_sys_id, np.id, mm.mc_tot_fxd_cst,
       'backfill_group_c_000470', 'backfill_group_c_000470'
FROM cost_product_parameter src
JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.param_code = 'MC_NAME' AND sp.deleted_at IS NULL
JOIN mst_parameter np ON np.param_code = 'TOTAL_FIXED_COST' AND np.deleted_at IS NULL
JOIN mst_machine mm ON mm.mc_name = src.cpp_value_text AND mm.deleted_at IS NULL
WHERE src.cpp_value_text IS NOT NULL
  AND mm.mc_tot_fxd_cst IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_parameter cpp
      WHERE cpp.cpp_product_sys_id = src.cpp_product_sys_id
        AND cpp.cpp_param_id       = np.id
  );

COMMIT;
