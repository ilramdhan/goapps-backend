-- 000514 — Re-backfill CAPP + CPP rows for the 6 Group-C params (repair of 000470).
--
-- WHY THIS EXISTS
-- The production ledger records 000468 / 000469 / 000470 as applied, but their
-- DML never landed (the trio was a DML-only commit, most likely skipped past with
-- `migrate force`). The schema is honest; the seed DATA is missing. Consequence:
-- the 6 Group-C params are not checklisted against any product, LoadCAPP's INNER
-- JOIN cost_product_applicable_param -> cost_product_parameter yields nothing, and
-- 10 cost-sheet export rows print "-".
-- Migrations are immutable once merged, so the repair lands here, not in 000470.
--
-- ORDERING DEPENDENCY (hard requirement)
--   000512 (re-seed of 000468) MUST have run before this file.
-- 000512 re-creates the 6 Group-C params in mst_parameter and re-wires their
-- lookup_master_code / lookup_fill_group_code / lookup_source_column. Without
-- those param definitions there is nothing to checklist against, so PRE-FLIGHT 1
-- below raises instead of writing zero rows.
--
-- WHAT THIS RESTORES (the effect of 000470, not its literal text)
--   PART 1  CAPP checklist rows for all 6 params, scoped to the products that
--           already carry the legacy source params (yarn cost model set).
--   PART 2  NS_LOSS_TYPE / BC_LOSS_TYPE — TEXT grade triggers copied verbatim
--           from STD_VALUE_LOSS / VALUE_LOSS.
--   PART 3  NS_LOSS / STD_SP_AX / STD_SP_BC — numeric children derived from
--           mst_product_grade via the grade-name trigger.
--   PART 4  TOTAL_FIXED_COST — numeric, derived from mst_machine via MC_NAME.
-- Products whose source column is NULL still get the CAPP row but no CPP value:
-- checklisted-but-unfilled, which the export prints as "-". Seeding a fabricated
-- 0 would be indistinguishable from a real zero cost.
--
-- DESIGN NOTES / DIFFERENCES FROM 000470
-- (a) ZERO literal product_sys_id. Product selection is entirely relational. The
--     VALUES list in PART 3 carries param CODE triples only, never product ids.
--     This is deliberate: 000464 hardcoded a product_sys_id range that does not
--     exist in production, its WHERE EXISTS guard failed wholesale, it inserted
--     zero rows, and its DO block passed anyway because it counted DEFINITIONS,
--     not ROWS. That failure mode must not repeat.
-- (b) Guards assert on ROW counts, and they assert the INHERITANCE SOURCE exists
--     BEFORE any INSERT. "Dynamic" guarantees no namespace mismatch; it does NOT
--     guarantee that any row lands. If cost_product_parameter holds no MC_NAME /
--     STD_VALUE_LOSS / VALUE_LOSS rows, this migration aborts loudly.
-- (c) NO hardcoded expected totals. 000470's header numbers (13,429 / 13,314 /
--     12,831) are self-declared "verified on dev" and cannot be trusted against
--     production — the same is true of every `-- VERIFIED` comment in this era of
--     migrations (000476:46 cites product 90299, which does not exist in prod).
--     Every post-condition here is RELATIONAL: "final row count for target X
--     equals the count of eligible SOURCE rows for X", computed in the same
--     transaction. It is therefore correct at any production scale.
-- (d) Idempotent. Every INSERT carries ON CONFLICT ON CONSTRAINT ... DO NOTHING
--     on the real unique constraints, and every post-condition asserts FINAL
--     STATE rather than inserted-row count, so a second run is a no-op that
--     still passes.
-- (e) Audit marker: 'rebackfill_group_c_000514' in capp_created_by /
--     cpp_created_by / cpp_filled_by. The .down.sql reverses ONLY those rows.
--
-- SCHEMA PROVENANCE (every name below was read, not guessed)
--   cost_product_parameter, cpp_unique_product_param (cpp_product_sys_id,
--     cpp_param_id), cpp_one_value_chk  -> 000217:12,29,32
--   cost_product_applicable_param, capp_unique_product_param
--     (capp_product_sys_id, capp_param_id)                 -> 000218:13,29
--   mst_parameter.id / param_code / deleted_at             -> 000004:2-4
--   mst_product_grade.pg_name                              -> 000387:7
--   mst_product_grade.std_selling_price / sp_value         -> 000405:4-5
--   mst_product_grade.loss_pct                             -> 000411:63
--   mst_machine.mc_name                                    -> 000384:7
--   mst_machine.mc_tot_fxd_cst                             -> 000423:5
--   the 6 Group-C param codes                              -> 000468:49-54

BEGIN;

-- ============================================================
-- PRE-FLIGHT 1: the 6 Group-C params must be defined (000512 ran)
-- ============================================================
DO $$
DECLARE
    v_defined INT;
    v_missing TEXT;
BEGIN
    SELECT COUNT(*) INTO v_defined
    FROM mst_parameter
    WHERE deleted_at IS NULL
      AND param_code IN ('TOTAL_FIXED_COST', 'NS_LOSS_TYPE', 'BC_LOSS_TYPE',
                         'NS_LOSS', 'STD_SP_AX', 'STD_SP_BC');

    RAISE NOTICE '000514 pre-flight: Group-C params defined = % / 6', v_defined;

    IF v_defined <> 6 THEN
        SELECT string_agg(c, ', ') INTO v_missing
        FROM unnest(ARRAY['TOTAL_FIXED_COST', 'NS_LOSS_TYPE', 'BC_LOSS_TYPE',
                          'NS_LOSS', 'STD_SP_AX', 'STD_SP_BC']) AS c
        WHERE NOT EXISTS (
            SELECT 1 FROM mst_parameter p
            WHERE p.param_code = c AND p.deleted_at IS NULL
        );
        RAISE EXCEPTION
            '000514 ABORT: expected 6 Group-C params in mst_parameter, found %. Missing: %. Migration 000512 (re-seed of 000468) must run BEFORE 000514.',
            v_defined, COALESCE(v_missing, '(duplicate param_code rows?)');
    END IF;
END $$;

-- ============================================================
-- PRE-FLIGHT 2: the INHERITANCE SOURCE rows must exist
-- ============================================================
-- Everything this migration writes is inherited from cost_product_parameter rows
-- carrying MC_NAME / STD_VALUE_LOSS / VALUE_LOSS. If any of the three is empty,
-- the corresponding INSERT would silently write zero rows — the exact 000464
-- failure mode. Abort instead.
DO $$
DECLARE
    v_mc   BIGINT;
    v_ns   BIGINT;
    v_bc   BIGINT;
    v_prod BIGINT;
    v_real BIGINT;
BEGIN
    SELECT
        COUNT(*) FILTER (WHERE sp.param_code = 'MC_NAME'),
        COUNT(*) FILTER (WHERE sp.param_code = 'STD_VALUE_LOSS'),
        COUNT(*) FILTER (WHERE sp.param_code = 'VALUE_LOSS'),
        COUNT(DISTINCT src.cpp_product_sys_id)
    INTO v_mc, v_ns, v_bc, v_prod
    FROM cost_product_parameter src
    JOIN mst_parameter sp
      ON sp.id = src.cpp_param_id
     AND sp.deleted_at IS NULL
    WHERE sp.param_code IN ('MC_NAME', 'STD_VALUE_LOSS', 'VALUE_LOSS');

    RAISE NOTICE '000514 pre-flight: source CPP rows MC_NAME=%, STD_VALUE_LOSS=%, VALUE_LOSS=%; distinct source products=%',
        v_mc, v_ns, v_bc, v_prod;

    -- Real vs migration-only database. The only products any migration inserts
    -- are the TXFX_% fixtures of 000236 / 000239, and no migration ever writes
    -- MC_NAME / STD_VALUE_LOSS / VALUE_LOSS values for them — those per-product
    -- values arrive exclusively from the application / Oracle import. So on a
    -- database built by migrations alone the inheritance source is legitimately
    -- empty, and aborting there would block the migration chain over an absence
    -- of data that is expected by construction.
    SELECT COUNT(*) INTO v_real
      FROM cost_product_master
     WHERE cpm_product_code NOT LIKE 'TXFX\_%';

    -- On a populated database an empty source IS the 000464 failure mode: every
    -- INSERT below writes zero rows while each statement still "succeeds", and
    -- the export keeps printing "-". Volume assertion, so it is conditional on
    -- the catalogue actually holding real products.
    IF v_real > 0 AND (v_mc = 0 OR v_ns = 0 OR v_bc = 0) THEN
        RAISE EXCEPTION
            '000514 ABORT: inheritance source is empty (MC_NAME=%, STD_VALUE_LOSS=%, VALUE_LOSS=%). Nothing can be derived; refusing to write zero rows silently. Check that the legacy per-product params were seeded before re-running.',
            v_mc, v_ns, v_bc;
    ELSIF v_mc = 0 OR v_ns = 0 OR v_bc = 0 THEN
        RAISE NOTICE
            '000514: inheritance source empty (MC_NAME=%, STD_VALUE_LOSS=%, VALUE_LOSS=%) and non-fixture products = % — nothing to derive. EXPECTED on a migration-only database such as CI; on production this would mean the Group-C columns export as "-".',
            v_mc, v_ns, v_bc, v_real;
    END IF;

    IF v_real > 0 AND v_prod = 0 THEN
        RAISE EXCEPTION
            '000514 ABORT: zero distinct products carry the Group-C source params. CAPP would be written for no product at all.';
    ELSIF v_prod = 0 THEN
        RAISE NOTICE
            '000514: zero distinct products carry the Group-C source params — EXPECTED on a migration-only database (non-fixture products = %).', v_real;
    END IF;
END $$;

-- ============================================================
-- PRE-FLIGHT 3: fan-out sanity on the lookup masters
-- ============================================================
-- PART 3 joins on mst_product_grade.pg_name and PART 4 on mst_machine.mc_name.
-- Neither has a unique index on the NAME column (only pg_code / mc_code are
-- unique — 000387:23, 000384:27), so duplicates would fan the join out. The
-- INSERTs below defend with DISTINCT ON; this block only reports the situation
-- so a surprising row count is explainable rather than mysterious.
DO $$
DECLARE
    v_dup_grade INT;
    v_dup_mc    INT;
BEGIN
    SELECT COUNT(*) INTO v_dup_grade FROM (
        SELECT pg_name FROM mst_product_grade WHERE deleted_at IS NULL
        GROUP BY pg_name HAVING COUNT(*) > 1
    ) d;
    SELECT COUNT(*) INTO v_dup_mc FROM (
        SELECT mc_name FROM mst_machine WHERE deleted_at IS NULL
        GROUP BY mc_name HAVING COUNT(*) > 1
    ) d;

    RAISE NOTICE '000514 pre-flight: duplicate active pg_name groups=%, duplicate active mc_name groups=%',
        v_dup_grade, v_dup_mc;
    IF v_dup_grade > 0 OR v_dup_mc > 0 THEN
        RAISE NOTICE '000514: duplicates present — DISTINCT ON (deterministic by pg_code / mc_code) selects one source row per product.';
    END IF;
END $$;

-- ============================================================
-- PART 1: Checklist all 6 params (CAPP)
-- ============================================================
-- Scope: the products that already carry the source params, i.e. the yarn cost
-- model set. Not a CROSS JOIN over cost_product_master — bought-out / non-costed
-- products stay untouched, matching the scoping decision made in 000469 PART 4.
DO $$
DECLARE
    v_ins BIGINT;
BEGIN
    INSERT INTO cost_product_applicable_param (
        capp_product_sys_id, capp_param_id,
        capp_is_required, capp_display_order, capp_created_by
    )
    SELECT DISTINCT src.cpp_product_sys_id, np.id, FALSE, NULL::INT,
           'rebackfill_group_c_000514'
    FROM cost_product_parameter src
    JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
    CROSS JOIN mst_parameter np
    WHERE sp.param_code IN ('MC_NAME', 'STD_VALUE_LOSS', 'VALUE_LOSS')
      AND np.param_code IN (
          'TOTAL_FIXED_COST', 'NS_LOSS_TYPE', 'BC_LOSS_TYPE',
          'NS_LOSS', 'STD_SP_AX', 'STD_SP_BC'
      )
      AND np.deleted_at IS NULL
    ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

    GET DIAGNOSTICS v_ins = ROW_COUNT;
    RAISE NOTICE '000514 PART 1: CAPP rows inserted = %', v_ins;
END $$;

-- ============================================================
-- PART 2: Grade trigger values (TEXT) — NS_LOSS_TYPE / BC_LOSS_TYPE
-- ============================================================
-- Copied verbatim from the legacy text params. cpp_one_value_chk (000217:32)
-- requires exactly one of numeric/text/flag, so only cpp_value_text is set.
DO $$
DECLARE
    v_ins BIGINT;
BEGIN
    INSERT INTO cost_product_parameter (
        cpp_product_sys_id, cpp_param_id, cpp_value_text,
        cpp_filled_by, cpp_created_by
    )
    SELECT src.cpp_product_sys_id, np.id, src.cpp_value_text,
           'rebackfill_group_c_000514', 'rebackfill_group_c_000514'
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
    ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;

    GET DIAGNOSTICS v_ins = ROW_COUNT;
    RAISE NOTICE '000514 PART 2: CPP text rows inserted (NS_LOSS_TYPE + BC_LOSS_TYPE) = %', v_ins;
END $$;

-- ============================================================
-- PART 3: Grade-derived numeric children
-- ============================================================
-- NS_LOSS follows the NS grade (STD_VALUE_LOSS); STD_SP_AX / STD_SP_BC follow
-- the BC grade (VALUE_LOSS) — matching the lookup_fill_group_code wiring in
-- 000468:76-83. Rows whose source column is NULL are skipped, leaving the param
-- checklisted but unfilled.
-- NOTE: the VALUES list holds PARAM CODE triples (source code, target code,
-- source column). It contains no product identifiers.
DO $$
DECLARE
    v_ins BIGINT;
BEGIN
    INSERT INTO cost_product_parameter (
        cpp_product_sys_id, cpp_param_id, cpp_value_numeric,
        cpp_filled_by, cpp_created_by
    )
    SELECT DISTINCT ON (s.cpp_product_sys_id, s.target_id)
           s.cpp_product_sys_id, s.target_id, s.val,
           'rebackfill_group_c_000514', 'rebackfill_group_c_000514'
    FROM (
        SELECT src.cpp_product_sys_id, np.id AS target_id, g.pg_code, m.val
        FROM (VALUES
            ('STD_VALUE_LOSS', 'NS_LOSS',   'loss_pct'),
            ('VALUE_LOSS',     'STD_SP_AX', 'std_selling_price'),
            ('VALUE_LOSS',     'STD_SP_BC', 'sp_value')
        ) AS map(source_code, target_code, source_col)
        JOIN mst_parameter sp ON sp.param_code = map.source_code AND sp.deleted_at IS NULL
        JOIN mst_parameter np ON np.param_code = map.target_code AND np.deleted_at IS NULL
        JOIN cost_product_parameter src
             ON src.cpp_param_id = sp.id AND src.cpp_value_text IS NOT NULL
        JOIN mst_product_grade g
             ON g.pg_name = src.cpp_value_text AND g.deleted_at IS NULL
        CROSS JOIN LATERAL (
            SELECT CASE map.source_col
                       WHEN 'loss_pct'          THEN g.loss_pct
                       WHEN 'std_selling_price' THEN g.std_selling_price
                       WHEN 'sp_value'          THEN g.sp_value
                   END AS val
        ) m
        WHERE m.val IS NOT NULL
    ) s
    ORDER BY s.cpp_product_sys_id, s.target_id, s.pg_code
    ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;

    GET DIAGNOSTICS v_ins = ROW_COUNT;
    RAISE NOTICE '000514 PART 3: CPP numeric rows inserted (NS_LOSS + STD_SP_AX + STD_SP_BC) = %', v_ins;
END $$;

-- ============================================================
-- PART 4: TOTAL_FIXED_COST from the machine master
-- ============================================================
-- MC_NAME holds the machine name as text per product; mc_tot_fxd_cst was added
-- by 000423:5. Machines with a NULL mc_tot_fxd_cst are skipped (CAPP row only).
DO $$
DECLARE
    v_ins BIGINT;
BEGIN
    INSERT INTO cost_product_parameter (
        cpp_product_sys_id, cpp_param_id, cpp_value_numeric,
        cpp_filled_by, cpp_created_by
    )
    SELECT DISTINCT ON (s.cpp_product_sys_id)
           s.cpp_product_sys_id, s.target_id, s.val,
           'rebackfill_group_c_000514', 'rebackfill_group_c_000514'
    FROM (
        SELECT src.cpp_product_sys_id, np.id AS target_id,
               mm.mc_code, mm.mc_tot_fxd_cst AS val
        FROM cost_product_parameter src
        JOIN mst_parameter sp
          ON sp.id = src.cpp_param_id AND sp.param_code = 'MC_NAME' AND sp.deleted_at IS NULL
        JOIN mst_parameter np
          ON np.param_code = 'TOTAL_FIXED_COST' AND np.deleted_at IS NULL
        JOIN mst_machine mm
          ON mm.mc_name = src.cpp_value_text AND mm.deleted_at IS NULL
        WHERE src.cpp_value_text IS NOT NULL
          AND mm.mc_tot_fxd_cst IS NOT NULL
    ) s
    ORDER BY s.cpp_product_sys_id, s.mc_code
    ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;

    GET DIAGNOSTICS v_ins = ROW_COUNT;
    RAISE NOTICE '000514 PART 4: CPP numeric rows inserted (TOTAL_FIXED_COST) = %', v_ins;
END $$;

-- ============================================================
-- POST-CONDITIONS: relational, no hardcoded totals
-- ============================================================
-- Each assertion compares the FINAL number of rows for a target param against
-- the number of ELIGIBLE SOURCE rows computed in this same transaction. Final
-- state (not inserted count) is asserted so a re-run stays green. Any shortfall
-- aborts the transaction.
DO $$
DECLARE
    v_src_prod   BIGINT;
    v_capp_have  BIGINT;
    v_capp_want  BIGINT;
    r            RECORD;
    v_have       BIGINT;
    v_real       BIGINT;
BEGIN
    -- Real (non-fixture) products: see PRE-FLIGHT 2 for why this is the
    -- populated-vs-migration-only discriminator.
    SELECT COUNT(*) INTO v_real
      FROM cost_product_master
     WHERE cpm_product_code NOT LIKE 'TXFX\_%';

    -- Products in scope: those carrying any of the three source params.
    SELECT COUNT(DISTINCT src.cpp_product_sys_id) INTO v_src_prod
    FROM cost_product_parameter src
    JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
    WHERE sp.param_code IN ('MC_NAME', 'STD_VALUE_LOSS', 'VALUE_LOSS');

    -- --- CAPP: every in-scope product must be checklisted for all 6 params ---
    v_capp_want := v_src_prod * 6;
    SELECT COUNT(*) INTO v_capp_have
    FROM cost_product_applicable_param capp
    JOIN mst_parameter np ON np.id = capp.capp_param_id AND np.deleted_at IS NULL
    WHERE np.param_code IN ('TOTAL_FIXED_COST', 'NS_LOSS_TYPE', 'BC_LOSS_TYPE',
                            'NS_LOSS', 'STD_SP_AX', 'STD_SP_BC')
      AND capp.capp_product_sys_id IN (
          SELECT src.cpp_product_sys_id
          FROM cost_product_parameter src
          JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
          WHERE sp.param_code IN ('MC_NAME', 'STD_VALUE_LOSS', 'VALUE_LOSS')
      );

    RAISE NOTICE '000514 post: in-scope products=%, CAPP rows for the 6 params=% (expected %)',
        v_src_prod, v_capp_have, v_capp_want;

    IF v_capp_have < v_capp_want THEN
        RAISE EXCEPTION
            '000514 ABORT: CAPP shortfall — % rows present for the 6 Group-C params across % in-scope products, expected % (6 per product). The checklist is incomplete, so LoadCAPP would still drop these params.',
            v_capp_have, v_src_prod, v_capp_want;
    END IF;

    -- --- CPP: one assertion per target param, source-derived expectation ---
    FOR r IN
        WITH expected AS (
            -- TEXT children: every source row with a non-null text value.
            SELECT 'NS_LOSS_TYPE'::TEXT AS target_code,
                   COUNT(DISTINCT src.cpp_product_sys_id) AS want
            FROM cost_product_parameter src
            JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
            WHERE sp.param_code = 'STD_VALUE_LOSS' AND src.cpp_value_text IS NOT NULL
            UNION ALL
            SELECT 'BC_LOSS_TYPE',
                   COUNT(DISTINCT src.cpp_product_sys_id)
            FROM cost_product_parameter src
            JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
            WHERE sp.param_code = 'VALUE_LOSS' AND src.cpp_value_text IS NOT NULL
            UNION ALL
            -- Grade-derived numerics: source rows whose grade column is non-null.
            SELECT 'NS_LOSS',
                   COUNT(DISTINCT src.cpp_product_sys_id)
            FROM cost_product_parameter src
            JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
            JOIN mst_product_grade g ON g.pg_name = src.cpp_value_text AND g.deleted_at IS NULL
            WHERE sp.param_code = 'STD_VALUE_LOSS' AND g.loss_pct IS NOT NULL
            UNION ALL
            SELECT 'STD_SP_AX',
                   COUNT(DISTINCT src.cpp_product_sys_id)
            FROM cost_product_parameter src
            JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
            JOIN mst_product_grade g ON g.pg_name = src.cpp_value_text AND g.deleted_at IS NULL
            WHERE sp.param_code = 'VALUE_LOSS' AND g.std_selling_price IS NOT NULL
            UNION ALL
            SELECT 'STD_SP_BC',
                   COUNT(DISTINCT src.cpp_product_sys_id)
            FROM cost_product_parameter src
            JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
            JOIN mst_product_grade g ON g.pg_name = src.cpp_value_text AND g.deleted_at IS NULL
            WHERE sp.param_code = 'VALUE_LOSS' AND g.sp_value IS NOT NULL
            UNION ALL
            -- Machine-derived numeric.
            SELECT 'TOTAL_FIXED_COST',
                   COUNT(DISTINCT src.cpp_product_sys_id)
            FROM cost_product_parameter src
            JOIN mst_parameter sp ON sp.id = src.cpp_param_id AND sp.deleted_at IS NULL
            JOIN mst_machine mm ON mm.mc_name = src.cpp_value_text AND mm.deleted_at IS NULL
            WHERE sp.param_code = 'MC_NAME' AND mm.mc_tot_fxd_cst IS NOT NULL
        ),
        actual AS (
            SELECT np.param_code AS target_code, COUNT(*) AS have
            FROM cost_product_parameter cpp
            JOIN mst_parameter np ON np.id = cpp.cpp_param_id AND np.deleted_at IS NULL
            WHERE np.param_code IN ('NS_LOSS_TYPE', 'BC_LOSS_TYPE', 'NS_LOSS',
                                    'STD_SP_AX', 'STD_SP_BC', 'TOTAL_FIXED_COST')
            GROUP BY np.param_code
        )
        SELECT e.target_code, e.want, COALESCE(a.have, 0) AS have
        FROM expected e
        LEFT JOIN actual a ON a.target_code = e.target_code
        ORDER BY e.target_code
    LOOP
        RAISE NOTICE '000514 post: CPP % -> % rows present, % eligible source rows',
            r.target_code, r.have, r.want;

        IF r.want > 0 AND r.have < r.want THEN
            RAISE EXCEPTION
                '000514 ABORT: CPP shortfall for % — % rows present but % eligible source rows exist. Rows were dropped or never written; refusing to commit a partial backfill.',
                r.target_code, r.have, r.want;
        END IF;

        IF r.want = 0 THEN
            RAISE WARNING
                '000514: zero eligible source rows for % — this param stays checklisted but unfilled and will export as "-". Verify the upstream master data.',
                r.target_code;
        END IF;
    END LOOP;

    -- Global floor: at least one value row must carry this migration's marker OR
    -- already exist from a prior run. Zero on both counts means a silent no-op.
    SELECT COUNT(*) INTO v_have
    FROM cost_product_parameter cpp
    JOIN mst_parameter np ON np.id = cpp.cpp_param_id AND np.deleted_at IS NULL
    WHERE np.param_code IN ('NS_LOSS_TYPE', 'BC_LOSS_TYPE', 'NS_LOSS',
                            'STD_SP_AX', 'STD_SP_BC', 'TOTAL_FIXED_COST');
    -- Volume assertion, not a correctness one: with an empty inheritance source
    -- (migration-only database) zero written rows is arithmetically correct, and
    -- the per-param shortfall loop above already proves correctness either way —
    -- every want is 0, so every have >= want holds for the right reason.
    IF v_real > 0 AND v_have = 0 THEN
        RAISE EXCEPTION
            '000514 ABORT: zero CPP value rows exist for the 6 Group-C params after the backfill. This is the 000464 silent-no-op failure mode; the export would still print "-".';
    ELSIF v_have = 0 THEN
        RAISE NOTICE
            '000514: zero Group-C CPP value rows after the backfill (non-fixture products = %) — EXPECTED on a migration-only database such as CI, since no migration ever seeds the MC_NAME / STD_VALUE_LOSS / VALUE_LOSS source values. On production this would be the 000464 silent no-op.', v_real;
    END IF;
    RAISE NOTICE '000514 post: total Group-C CPP value rows = %', v_have;
END $$;

COMMIT;
