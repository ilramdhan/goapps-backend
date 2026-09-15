-- 000512: Re-seed the Group-C master data that 000468 was supposed to install.
--
-- WHY THIS EXISTS
-- The production ledger schema_migrations_finance reports version 509, so every
-- schema migration is applied. But 000468 / 000469 / 000470 are DML-only and
-- their DATA is absent in production: the 6 Group-C params below do not exist,
-- so the cost-sheet export prints "-" for the rows that read them. The most
-- likely cause is a `migrate force` that advanced the ledger past this commit
-- without running it (NOT PROVEN — golang-migrate v4.18.1 keeps no timestamps).
-- Migrations are immutable once merged, so the repair lands here, not in 000468.
--
-- SCOPE: this file restores the effects of 000468 ONLY (params + lookup wiring).
-- The CAPP/CPP backfill of 000470 and the formulas of 000469 are separate files.
--
-- WHAT 000468 DID (mirrored here verbatim in effect):
--   CSV row  9 Total Fixed Cost -> mst_machine.mc_tot_fxd_cst
--   CSV row 73 STD SP AX        -> mst_product_grade.std_selling_price
--   CSV row 74 STD SP BC        -> mst_product_grade.sp_value
--   plus the two PRODUCT_GRADE trigger params (NS_LOSS_TYPE / BC_LOSS_TYPE)
--   and the numeric NS_LOSS child.
--
-- DIFFERENCES FROM 000468 (deliberate):
--   1. Audit marker is 'reseed_group_c_000512', never 'wire_group_c_000468', so
--      000468's own .down.sql cannot delete these rows and an auditor can tell
--      the re-seed apart from the original run.
--   2. A verification block asserts on ROW COUNTS, not on definition counts.
--      000464 wrote zero rows in production and its DO block passed anyway
--      because it only counted definitions. That must not repeat here.
--   3. The UOM resolution is asserted. 000468 used a LEFT JOIN on mst_uom, so a
--      missing 'USD' row would silently produce params with uom_id NULL.
--   4. No literal product_sys_id appears anywhere in this file (000468 had none
--      either); nothing here selects a product at all.
--
-- The assert constants (6 / 2 / 4 / 3) are counts of the literals written in
-- THIS file. No coverage number from an older migration's comments is used as an
-- assertion, because those were measured on dev, not production.

BEGIN;

-- ============================================================
-- PART 0: Preconditions
-- ============================================================
-- Fail loudly if the USD unit of measure is missing (seeded by 000374), rather
-- than silently inserting params with uom_id NULL.
DO $$
DECLARE
    v_usd INT;
BEGIN
    SELECT COUNT(*) INTO v_usd
      FROM mst_uom
     WHERE uom_code = 'USD' AND deleted_at IS NULL;

    RAISE NOTICE '000512 precondition: mst_uom USD rows = %', v_usd;

    IF v_usd <> 1 THEN
        RAISE EXCEPTION '000512: expected exactly 1 non-deleted mst_uom row with uom_code=''USD'', found %. Run 000374 first.', v_usd;
    END IF;
END $$;

-- ============================================================
-- PART 1: Insert the 6 Group-C params (idempotent)
-- ============================================================
-- Skips any param_code that already exists and is not soft-deleted, so this is
-- safe if 000468 turns out to have partially landed.

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, min_value, max_value, display_group, display_order,
    is_active, created_at, created_by
)
SELECT
    p.code, p.name, p.short_name, p.data_type, p.category,
    u.uom_id, p.default_val::NUMERIC, p.min_val::NUMERIC, p.max_val::NUMERIC,
    p.display_group, p.display_order, TRUE,
    NOW(), 'reseed_group_c_000512'
FROM (VALUES
  -- Fixed Cost group: CSV row 9, sibling of POWER_PER_DAY / MANPOWER_PER_DAY.
  ('TOTAL_FIXED_COST','Total Fixed Cost','Total Fixed Cost','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL,'Fixed Cost',81),
  -- Quality Loss group: CSV rows 70-74.
  ('NS_LOSS_TYPE','NS Loss Type','NS Loss Type','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL,'Quality Loss',143),
  ('BC_LOSS_TYPE','BC Loss Type','BC Loss Type','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL,'Quality Loss',144),
  ('NS_LOSS','NS Loss','NS Loss','NUMBER','MASTER_LOOKUP',NULL,NULL,NULL,NULL,'Quality Loss',145),
  ('STD_SP_AX','STD SP AX','STD SP AX','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL,'Quality Loss',146),
  ('STD_SP_BC','STD SP BC','STD SP BC','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL,'Quality Loss',147)
) AS p(code, name, short_name, data_type, category, uom_code, default_val, min_val, max_val, display_group, display_order)
LEFT JOIN mst_uom u ON u.uom_code = p.uom_code AND u.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Lookup triggers (lookup_master_code)
-- ============================================================
-- Both grade triggers source mst_product_grade (master registered by 000394).
-- The IS DISTINCT FROM guard keeps this a no-op on re-run and avoids stamping
-- updated_by on rows that already carry the right value.

UPDATE mst_parameter
   SET lookup_master_code = 'PRODUCT_GRADE',
       updated_at = NOW(),
       updated_by = 'reseed_group_c_000512'
 WHERE param_code IN ('NS_LOSS_TYPE', 'BC_LOSS_TYPE')
   AND deleted_at IS NULL
   AND lookup_master_code IS DISTINCT FROM 'PRODUCT_GRADE';

-- ============================================================
-- PART 3: Fill-group children (lookup_fill_group_code + lookup_source_column)
-- ============================================================

UPDATE mst_parameter p
   SET lookup_fill_group_code = w.fill_group,
       lookup_source_column   = w.source_col,
       updated_at = NOW(),
       updated_by = 'reseed_group_c_000512'
FROM (VALUES
  -- MC_NAME child — mirrors POWER_PER_DAY / MANPOWER_PER_DAY / OVERHEAD_PER_HEAD.
  ('TOTAL_FIXED_COST', 'MC_NAME',      'mc_tot_fxd_cst'),
  -- NS_LOSS_TYPE child.
  ('NS_LOSS',          'NS_LOSS_TYPE', 'loss_pct'),
  -- BC_LOSS_TYPE children.
  ('STD_SP_AX',        'BC_LOSS_TYPE', 'std_selling_price'),
  ('STD_SP_BC',        'BC_LOSS_TYPE', 'sp_value')
) AS w(code, fill_group, source_col)
WHERE p.param_code = w.code
  AND p.deleted_at IS NULL
  AND (p.lookup_fill_group_code IS DISTINCT FROM w.fill_group
    OR p.lookup_source_column   IS DISTINCT FROM w.source_col);

-- ============================================================
-- PART 4: Verification — asserts on ROW COUNTS, not on definitions
-- ============================================================
DO $$
DECLARE
    v_present       INT;
    v_mine          INT;
    v_trigger       INT;
    v_children      INT;
    v_usd_bound     INT;
    v_active        INT;
BEGIN
    -- All 6 params must physically exist as non-deleted rows.
    SELECT COUNT(*) INTO v_present
      FROM mst_parameter
     WHERE deleted_at IS NULL
       AND param_code IN ('TOTAL_FIXED_COST','NS_LOSS_TYPE','BC_LOSS_TYPE',
                          'NS_LOSS','STD_SP_AX','STD_SP_BC');

    -- How many of them this migration actually wrote (0 is legitimate only if
    -- 000468 already landed — reported, not asserted).
    SELECT COUNT(*) INTO v_mine
      FROM mst_parameter
     WHERE created_by = 'reseed_group_c_000512' AND deleted_at IS NULL;

    SELECT COUNT(*) INTO v_active
      FROM mst_parameter
     WHERE deleted_at IS NULL AND is_active = TRUE
       AND param_code IN ('TOTAL_FIXED_COST','NS_LOSS_TYPE','BC_LOSS_TYPE',
                          'NS_LOSS','STD_SP_AX','STD_SP_BC');

    SELECT COUNT(*) INTO v_trigger
      FROM mst_parameter
     WHERE deleted_at IS NULL
       AND param_code IN ('NS_LOSS_TYPE','BC_LOSS_TYPE')
       AND lookup_master_code = 'PRODUCT_GRADE';

    SELECT COUNT(*) INTO v_children
      FROM mst_parameter p
      JOIN (VALUES
        ('TOTAL_FIXED_COST', 'MC_NAME',      'mc_tot_fxd_cst'),
        ('NS_LOSS',          'NS_LOSS_TYPE', 'loss_pct'),
        ('STD_SP_AX',        'BC_LOSS_TYPE', 'std_selling_price'),
        ('STD_SP_BC',        'BC_LOSS_TYPE', 'sp_value')
      ) AS w(code, fill_group, source_col)
        ON p.param_code            = w.code
       AND p.lookup_fill_group_code = w.fill_group
       AND p.lookup_source_column   = w.source_col
     WHERE p.deleted_at IS NULL;

    -- The three USD-denominated params must have a resolved uom_id. A LEFT JOIN
    -- miss would leave these NULL without any error.
    SELECT COUNT(*) INTO v_usd_bound
      FROM mst_parameter
     WHERE deleted_at IS NULL
       AND param_code IN ('TOTAL_FIXED_COST','STD_SP_AX','STD_SP_BC')
       AND uom_id IS NOT NULL;

    RAISE NOTICE '000512: params present=% (expected 6), inserted_by_this_migration=%, active=%, grade_triggers=% (expected 2), fill_group_children=% (expected 4), usd_bound=% (expected 3)',
        v_present, v_mine, v_active, v_trigger, v_children, v_usd_bound;

    IF v_present <> 6 THEN
        RAISE EXCEPTION '000512: expected 6 non-deleted Group-C params, found % — the INSERT wrote no usable rows.', v_present;
    END IF;

    IF v_active <> 6 THEN
        RAISE EXCEPTION '000512: expected all 6 Group-C params to be is_active=TRUE, found %.', v_active;
    END IF;

    IF v_trigger <> 2 THEN
        RAISE EXCEPTION '000512: expected 2 params wired to lookup_master_code=''PRODUCT_GRADE'', found %.', v_trigger;
    END IF;

    IF v_children <> 4 THEN
        RAISE EXCEPTION '000512: expected 4 fill-group children with matching lookup_source_column, found %.', v_children;
    END IF;

    IF v_usd_bound <> 3 THEN
        RAISE EXCEPTION '000512: expected 3 USD-denominated params with a non-NULL uom_id, found % — mst_uom lookup failed.', v_usd_bound;
    END IF;
END $$;

COMMIT;
