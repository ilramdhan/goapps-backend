-- 000548 — seed the ERP rule master (design Part 1 §4.1, §6.6; plan-01 P0-T2).
--
-- Source: the lead's draft goapps_000392_erp_rules_seed.sql, taken read-only
-- from ALTHARADEV on 2026-09-25:
--   * MGTDAT.MGT_ITEM_COST_VAL_LOSS            -> 115 cst_erp_valloss_rule rows
--                                                 (COST 47, SPPTY 62, SPBSD 3, SPITY 3)
--   * IM_VS_STATIC_VALUE 'ITEMSELLPRIC'        ->   3 cst_erp_sell_price rows
--   * OM_GRADE_CODE_1.GRADE_BL_SHORT_NAME      ->  22 cst_erp_grade_group_seed rows
-- Before the PROD deploy, compare against PROD (recon_queries.sql §0); on any
-- difference, replace this seed rather than hand-editing prod rows.
--
-- Every seeded row carries created_by = 'migration:000548' (the seed marker).
-- The down migration deletes only marker rows.
--
-- Grade groups: the mapping lands in cst_erp_grade_group_seed and is applied
-- to any cost_erp_grade row that already exists, filling NULLs only (a group
-- set by Costing is never overwritten). Prod cost_erp_grade is EMPTY
-- (sql-1 F-2), so 0 applied rows is the expected prod outcome: that is a
-- RAISE NOTICE, never an error (S-6). P0-T15b re-applies the mapping after
-- the first ERP master sync. Grades with no group in ERP (CI, FIN, GEN, MB,
-- MC, NA, R, ...) stay NULL, and V-08 blocks them if they appear as non-AX ADJ.
--
-- The DO block RAISEs (aborting the whole migration) unless exactly
-- 115 / 3 / 22 marker rows exist, or if a non-COST basis has no price.

BEGIN;

INSERT INTO cst_erp_grade_group_seed (cggs_grade_code, cggs_grade_group, created_by) VALUES
    ('A9/A', 'NS', 'migration:000548'),
    ('A9', 'NS', 'migration:000548'),
    ('AE', 'AE', 'migration:000548'),
    ('AM', 'NS', 'migration:000548'),
    ('APQ', 'BC', 'migration:000548'),
    ('AXa', 'POYA', 'migration:000548'),
    ('AXb', 'POYA', 'migration:000548'),
    ('AXc', 'POYA', 'migration:000548'),
    ('AXd', 'POYA', 'migration:000548'),
    ('AX', 'AX', 'migration:000548'),
    ('Aa', 'POYA', 'migration:000548'),
    ('Ab', 'POYA', 'migration:000548'),
    ('Ac', 'POYA', 'migration:000548'),
    ('Ad', 'POYA', 'migration:000548'),
    ('A', 'NS', 'migration:000548'),
    ('B1', 'BC', 'migration:000548'),
    ('B2', 'BC', 'migration:000548'),
    ('BB', 'BB', 'migration:000548'),
    ('BMX', 'BC', 'migration:000548'),
    ('B', 'BC', 'migration:000548'),
    ('C', 'BC', 'migration:000548'),
    ('JLT', 'JLT', 'migration:000548')
ON CONFLICT (cggs_grade_code) DO NOTHING;

INSERT INTO cst_erp_sell_price (cesp_basis, cesp_price, created_by) VALUES
    ('SPBSD', 1.4, 'migration:000548'),
    ('SPITY', 1.5, 'migration:000548'),
    ('SPPTY', 1.3, 'migration:000548')
ON CONFLICT (cesp_basis) DO NOTHING;

INSERT INTO cst_erp_valloss_rule
    (cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss, created_by) VALUES
    ('Type 1', 'POY', 'BC', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 1', 'POY', 'JLT', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 1', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 1', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 1', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 1', 'PTY', 'BC', 'SPPTY', 0.1, 'migration:000548'),
    ('Type 1', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 1', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 10', 'ITY', 'AE', 'COST', 0.05, 'migration:000548'),
    ('Type 10', 'ITY', 'BB', 'SPITY', 0.6, 'migration:000548'),
    ('Type 10', 'ITY', 'BC', 'SPITY', 0.6, 'migration:000548'),
    ('Type 10', 'ITY', 'JLT', 'SPITY', 0.6, 'migration:000548'),
    ('Type 10', 'ITY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 10', 'POY', 'BC', 'COST', 0.0, 'migration:000548'),
    ('Type 10', 'POY', 'JLT', 'COST', 0.0, 'migration:000548'),
    ('Type 10', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 10', 'PTY', 'AE', 'COST', 0.05, 'migration:000548'),
    ('Type 10', 'PTY', 'BB', 'SPPTY', 0.6, 'migration:000548'),
    ('Type 10', 'PTY', 'BC', 'SPPTY', 0.6, 'migration:000548'),
    ('Type 10', 'PTY', 'JLT', 'SPPTY', 0.6, 'migration:000548'),
    ('Type 10', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 11', 'POY', 'BC', 'SPPTY', 0.45, 'migration:000548'),
    ('Type 11', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 11', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 11', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 11', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 11', 'PTY', 'BC', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 11', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 11', 'PTY', 'NS', 'COST', 0.0, 'migration:000548'),
    ('Type 12', 'POY', 'BB', 'COST', 0.5, 'migration:000548'),
    ('Type 12', 'POY', 'BC', 'COST', 0.5, 'migration:000548'),
    ('Type 12', 'POY', 'JLT', 'COST', 0.5, 'migration:000548'),
    ('Type 12', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 12', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 12', 'PTY', 'BB', 'SPBSD', 0.4, 'migration:000548'),
    ('Type 12', 'PTY', 'BC', 'SPBSD', 0.1, 'migration:000548'),
    ('Type 12', 'PTY', 'JLT', 'SPBSD', 0.4, 'migration:000548'),
    ('Type 12', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 13', 'POY', 'BC', 'SPPTY', 0.0, 'migration:000548'),
    ('Type 13', 'POY', 'JLT', 'SPPTY', 0.0, 'migration:000548'),
    ('Type 13', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 13', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 13', 'PTY', 'BB', 'SPPTY', 0.51, 'migration:000548'),
    ('Type 13', 'PTY', 'BC', 'SPPTY', 0.51, 'migration:000548'),
    ('Type 13', 'PTY', 'JLT', 'SPPTY', 0.51, 'migration:000548'),
    ('Type 13', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 2', 'POY', 'BB', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 2', 'POY', 'BC', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 2', 'POY', 'JLT', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 2', 'POY', 'NS', 'COST', 0.0, 'migration:000548'),
    ('Type 2', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 2', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 2', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 2', 'PTY', 'BC', 'SPPTY', 0.25, 'migration:000548'),
    ('Type 2', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 2', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 3', 'POY', 'BB', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 3', 'POY', 'BC', 'SPPTY', 0.45, 'migration:000548'),
    ('Type 3', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 3', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 3', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 3', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 3', 'PTY', 'BC', 'SPPTY', 0.0, 'migration:000548'),
    ('Type 3', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 3', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 4', 'POY', 'BB', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 4', 'POY', 'BC', 'SPPTY', 0.45, 'migration:000548'),
    ('Type 4', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 4', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 4', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 4', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 4', 'PTY', 'BC', 'SPPTY', 0.0, 'migration:000548'),
    ('Type 4', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 4', 'PTY', 'NS', 'COST', 0.0, 'migration:000548'),
    ('Type 5', 'POY', 'BC', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 5', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 5', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 5', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 5', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 5', 'PTY', 'BC', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 5', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 5', 'PTY', 'NS', 'COST', 0.05, 'migration:000548'),
    ('Type 6', 'POY', 'BC', 'SPPTY', 0.45, 'migration:000548'),
    ('Type 6', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 6', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 6', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 6', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 6', 'PTY', 'BC', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 6', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 6', 'PTY', 'NS', 'COST', 0.0, 'migration:000548'),
    ('Type 7', 'POY', 'BB', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 7', 'POY', 'BC', 'SPPTY', 0.45, 'migration:000548'),
    ('Type 7', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 7', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 7', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 7', 'PTY', 'BB', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 7', 'PTY', 'BC', 'SPPTY', 0.3, 'migration:000548'),
    ('Type 7', 'PTY', 'JLT', 'SPPTY', 0.4, 'migration:000548'),
    ('Type 7', 'PTY', 'NS', 'COST', 0.0, 'migration:000548'),
    ('Type 8', 'POY', 'BC', 'SPPTY', 0.45, 'migration:000548'),
    ('Type 8', 'POY', 'JLT', 'SPPTY', 0.8, 'migration:000548'),
    ('Type 8', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 8', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 8', 'PTY', 'BB', 'SPPTY', 0.6, 'migration:000548'),
    ('Type 8', 'PTY', 'BC', 'SPPTY', 0.6, 'migration:000548'),
    ('Type 8', 'PTY', 'JLT', 'SPPTY', 0.6, 'migration:000548'),
    ('Type 8', 'PTY', 'NS', 'COST', 0.0, 'migration:000548'),
    ('Type 9', 'POY', 'BC', 'SPPTY', 0.0, 'migration:000548'),
    ('Type 9', 'POY', 'JLT', 'SPPTY', 0.0, 'migration:000548'),
    ('Type 9', 'POY', 'POYA', 'COST', 0.0, 'migration:000548'),
    ('Type 9', 'PTY', 'AE', 'COST', 0.0, 'migration:000548'),
    ('Type 9', 'PTY', 'BB', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 9', 'PTY', 'BC', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 9', 'PTY', 'JLT', 'SPPTY', 0.5, 'migration:000548'),
    ('Type 9', 'PTY', 'NS', 'COST', 0.0, 'migration:000548')
ON CONFLICT (cevr_fg_type, cevr_prod_type, cevr_grade_group) WHERE cevr_is_active DO NOTHING;

-- Apply the mapping to existing replica rows (NULLs only).
UPDATE cost_erp_grade g
   SET ceg_grade_group = s.cggs_grade_group
  FROM cst_erp_grade_group_seed s
 WHERE g.ceg_grade_code = s.cggs_grade_code
   AND g.ceg_grade_group IS NULL;

DO $$
DECLARE
    v_rules   INT;
    v_prices  INT;
    v_groups  INT;
    v_orphan  INT;
    v_applied INT;
    v_missing TEXT;
BEGIN
    SELECT COUNT(*) INTO v_rules  FROM cst_erp_valloss_rule     WHERE created_by = 'migration:000548';
    SELECT COUNT(*) INTO v_prices FROM cst_erp_sell_price       WHERE created_by = 'migration:000548';
    SELECT COUNT(*) INTO v_groups FROM cst_erp_grade_group_seed WHERE created_by = 'migration:000548';

    IF v_rules <> 115 THEN
        RAISE EXCEPTION '000548: expected 115 cst_erp_valloss_rule seed rows, found %', v_rules;
    END IF;
    IF v_prices <> 3 THEN
        RAISE EXCEPTION '000548: expected 3 cst_erp_sell_price seed rows, found %', v_prices;
    END IF;
    IF v_groups <> 22 THEN
        RAISE EXCEPTION '000548: expected 22 cst_erp_grade_group_seed rows, found %', v_groups;
    END IF;

    SELECT COUNT(*) INTO v_orphan
      FROM cst_erp_valloss_rule r
     WHERE r.cevr_basis <> 'COST'
       AND NOT EXISTS (SELECT 1 FROM cst_erp_sell_price p WHERE p.cesp_basis = r.cevr_basis);
    IF v_orphan > 0 THEN
        RAISE EXCEPTION '000548: % valloss rule(s) use a basis with no cst_erp_sell_price row', v_orphan;
    END IF;

    -- Informational only (S-6): prod cost_erp_grade is empty at deploy time.
    SELECT COUNT(*) INTO v_applied
      FROM cost_erp_grade g
      JOIN cst_erp_grade_group_seed s ON s.cggs_grade_code = g.ceg_grade_code
     WHERE g.ceg_grade_group = s.cggs_grade_group;
    SELECT string_agg(s.cggs_grade_code, ', ' ORDER BY s.cggs_grade_code) INTO v_missing
      FROM cst_erp_grade_group_seed s
     WHERE NOT EXISTS (SELECT 1 FROM cost_erp_grade g WHERE g.ceg_grade_code = s.cggs_grade_code);

    RAISE NOTICE '000548: % cost_erp_grade row(s) carry the seeded group; P0-T15b re-applies after the master sync', v_applied;
    IF v_missing IS NOT NULL THEN
        RAISE NOTICE '000548: seed grade(s) not in cost_erp_grade yet: %', v_missing;
    END IF;
END $$;

COMMIT;
