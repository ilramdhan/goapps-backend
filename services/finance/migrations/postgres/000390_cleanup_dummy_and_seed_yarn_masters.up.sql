-- 000390: Remove old dummy/test params+formulas; seed MB Head, MB Spin, Box/Bobbin Cost.
-- Context: prior seeds (000234, 000240, bare 'seed') were dev prototypes, not Oracle yarn data.
--          They blocked seed_000382's F_YARN_STAGE_OUT terminal formula via result_param_id clash.
-- Strategy:
--   1. Delete formula_param rows for old formulas (no FK cascade defined).
--   2. Soft-delete old non-yarn formulas (22 rows).
--   3. Re-attribute WASTE_PCT + YARN_DENIER to seed_000381 (FK-safe — UUID unchanged).
--   4. Soft-delete all remaining old seed/test params (120 rows).
--   5. Re-insert F_YARN_STAGE_OUT (terminal formula, was blocked by old result_param_id).
--   6. Seed mst_mb_head, mst_mb_spin, mst_box_bobbin_cost.

BEGIN;

-- ─── 1. Remove formula_param for old non-yarn formulas ───────────────────────
DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula
    WHERE created_by IN ('seed', 'seed_000235', 'seed_000241')
      AND deleted_at IS NULL
);

-- ─── 2. Soft-delete old non-yarn formulas ────────────────────────────────────
UPDATE mst_formula
SET deleted_at = NOW(), deleted_by = 'migration_000390'
WHERE created_by IN ('seed', 'seed_000235', 'seed_000241')
  AND deleted_at IS NULL;

-- ─── 3. Re-attribute WASTE_PCT + YARN_DENIER (used by seed_000382 formula_param FKs) ──
-- Cannot change UUID (would break formula_param.param_id FK), so just relabel.
UPDATE mst_parameter
SET created_by       = 'seed_000381',
    param_name       = CASE param_code
                         WHEN 'WASTE_PCT'   THEN 'Waste Percentage (%)'
                         WHEN 'YARN_DENIER' THEN 'Yarn Nominal Denier'
                       END,
    param_short_name = CASE param_code
                         WHEN 'WASTE_PCT'   THEN 'Waste %'
                         WHEN 'YARN_DENIER' THEN 'Denier'
                       END
WHERE param_code IN ('WASTE_PCT', 'YARN_DENIER')
  AND created_by = 'seed'
  AND deleted_at IS NULL;

-- ─── 4. Soft-delete all remaining old dummy/test params ──────────────────────
UPDATE mst_parameter
SET deleted_at = NOW(), deleted_by = 'migration_000390'
WHERE created_by = 'seed'
  AND deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(), deleted_by = 'migration_000390'
WHERE created_by = 'seed_000234'
  AND deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(), deleted_by = 'migration_000390'
WHERE created_by = 'seed_000240'
  AND deleted_at IS NULL;

UPDATE mst_parameter
SET deleted_at = NOW(), deleted_by = 'migration_000390'
WHERE created_by IN ('admin', 'e2e_tester', 'loader-test', 'process-chunk-test')
  AND deleted_at IS NULL;

-- ─── 5. Re-insert F_YARN_STAGE_OUT (terminal formula, was blocked) ───────────
-- Old F_COST_STAGE_OUT (seed) held COST_STAGE_OUT as result_param_id and blocked
-- seed_000382's F_YARN_STAGE_OUT. Now that old formula is soft-deleted, insert it.
INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, created_by
)
SELECT
    'F_YARN_STAGE_OUT',
    'Terminal engine sink',
    'CALCULATION',
    'COST_DEL_FINAL',
    p.id,
    'Passthrough: COST_STAGE_OUT = COST_DEL_FINAL. Required by ScopeKeyFinalCost.',
    'seed_000382'
FROM mst_parameter p
WHERE p.param_code = 'COST_STAGE_OUT'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula f
      WHERE f.formula_code = 'F_YARN_STAGE_OUT' AND f.deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula fchk
      WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL
  );

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 0
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'COST_DEL_FINAL' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_STAGE_OUT'
  AND f.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM formula_param fp
      WHERE fp.formula_id = f.id AND fp.param_id = p.id
  );

-- ─── 6a. Seed MB Head master data ────────────────────────────────────────────
-- mbh_mb_costing = Oracle cost name (unique key used in system)
-- mbh_mgt_name   = human-readable management name
-- mbh_dozing     = oil dozing rate (lubricant application, typically 0.002-0.008)
INSERT INTO mst_mb_head (
    mbh_oracle_sys_id, mbh_mb_costing, mbh_mgt_name,
    mbh_denier, mbh_filament, mbh_dozing, mbh_is_active, created_by
)
SELECT
    v.oracle_sys_id, v.mb_costing, v.mgt_name,
    v.denier, v.filament, v.dozing, TRUE, 'seed_000390'
FROM (VALUES
    ('SYS-MBH-001', 'DTY-75D-36F-HEAD',  'DTY 75D/36F Texturing Head',    75.00,  36, 0.0035),
    ('SYS-MBH-002', 'DTY-100D-36F-HEAD', 'DTY 100D/36F Texturing Head',  100.00,  36, 0.0040),
    ('SYS-MBH-003', 'DTY-150D-48F-HEAD', 'DTY 150D/48F Texturing Head',  150.00,  48, 0.0045),
    ('SYS-MBH-004', 'DTY-167D-48F-HEAD', 'DTY 167D/48F Texturing Head',  167.00,  48, 0.0050),
    ('SYS-MBH-005', 'POY-75D-36F-HEAD',  'POY 75D/36F Spin Head',         75.00,  36, 0.0020),
    ('SYS-MBH-006', 'POY-150D-48F-HEAD', 'POY 150D/48F Spin Head',       150.00,  48, 0.0025),
    ('SYS-MBH-007', 'PTY-50D-24F-HEAD',  'PTY 50D/24F Poly Head',         50.00,  24, 0.0030),
    ('SYS-MBH-008', 'FDY-75D-36F-HEAD',  'FDY 75D/36F Full-Draw Head',    75.00,  36, 0.0022)
) AS v(oracle_sys_id, mb_costing, mgt_name, denier, filament, dozing)
WHERE NOT EXISTS (
    SELECT 1 FROM mst_mb_head WHERE mbh_oracle_sys_id = v.oracle_sys_id
);

-- ─── 6b. Seed MB Spin master data (linked to MB Head via FK) ─────────────────
INSERT INTO mst_mb_spin (
    mbs_oracle_sys_id, mbs_mbh_id, mbs_mgt_name,
    mbs_denier, mbs_filament, mbs_dozing, mbs_mb_costing, mbs_is_active, created_by
)
SELECT
    v.oracle_sys_id, h.mbh_id, v.mgt_name,
    v.denier, v.filament, v.dozing, v.mb_costing, TRUE, 'seed_000390'
FROM (VALUES
    ('SYS-MBS-001', 'DTY-75D-36F-HEAD',  'DTY 75D/36F-A Spin Beam',   75.00,  36, 0.0035, 'DTY-75D-36F-SPIN-A'),
    ('SYS-MBS-002', 'DTY-75D-36F-HEAD',  'DTY 75D/36F-B Spin Beam',   75.00,  36, 0.0033, 'DTY-75D-36F-SPIN-B'),
    ('SYS-MBS-003', 'DTY-100D-36F-HEAD', 'DTY 100D/36F-A Spin Beam', 100.00,  36, 0.0040, 'DTY-100D-36F-SPIN-A'),
    ('SYS-MBS-004', 'DTY-150D-48F-HEAD', 'DTY 150D/48F-A Spin Beam', 150.00,  48, 0.0045, 'DTY-150D-48F-SPIN-A'),
    ('SYS-MBS-005', 'DTY-167D-48F-HEAD', 'DTY 167D/48F-A Spin Beam', 167.00,  48, 0.0050, 'DTY-167D-48F-SPIN-A'),
    ('SYS-MBS-006', 'POY-75D-36F-HEAD',  'POY 75D/36F-A Spin Beam',   75.00,  36, 0.0020, 'POY-75D-36F-SPIN-A'),
    ('SYS-MBS-007', 'POY-150D-48F-HEAD', 'POY 150D/48F-A Spin Beam', 150.00,  48, 0.0025, 'POY-150D-48F-SPIN-A'),
    ('SYS-MBS-008', 'PTY-50D-24F-HEAD',  'PTY 50D/24F-A Spin Beam',   50.00,  24, 0.0030, 'PTY-50D-24F-SPIN-A'),
    ('SYS-MBS-009', 'FDY-75D-36F-HEAD',  'FDY 75D/36F-A Spin Beam',   75.00,  36, 0.0022, 'FDY-75D-36F-SPIN-A')
) AS v(oracle_sys_id, head_costing, mgt_name, denier, filament, dozing, mb_costing)
JOIN mst_mb_head h ON h.mbh_mb_costing = v.head_costing AND h.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_mb_spin WHERE mbs_oracle_sys_id = v.oracle_sys_id
);

-- ─── 6c. Seed Box/Bobbin Cost master data ────────────────────────────────────
-- bbc_type: CAP = captive market, DEL = delivery/export market
-- no_of_bob: bobbins per carton (drives per-kg packaging cost in formula chain)
INSERT INTO mst_box_bobbin_cost (
    bbc_code, bbc_name, bbc_type, no_of_bob, is_active, notes, created_by
)
SELECT v.code, v.name, v.bbc_type, v.no_of_bob, TRUE, v.notes, 'seed_000390'
FROM (VALUES
    ('CAP-STD-6',   'Captive Standard Box (6 Bob)',   'CAP',  6, 'Standard captive packaging — 6 bobbins per carton'),
    ('CAP-STD-4',   'Captive Large Box (4 Bob)',      'CAP',  4, 'Large-bobbin captive packaging — 4 per carton'),
    ('CAP-MINI-12', 'Captive Mini Box (12 Bob)',      'CAP', 12, 'Small-bobbin captive packaging — 12 per carton'),
    ('DEL-STD-6',   'Delivery Standard Box (6 Bob)', 'DEL',  6, 'Standard export delivery packaging — 6 bobbins'),
    ('DEL-STD-4',   'Delivery Large Box (4 Bob)',    'DEL',  4, 'Large-bobbin export packaging — 4 per carton'),
    ('DEL-MINI-12', 'Delivery Mini Box (12 Bob)',    'DEL', 12, 'Small-bobbin export packaging — 12 per carton')
) AS v(code, name, bbc_type, no_of_bob, notes)
WHERE NOT EXISTS (
    SELECT 1 FROM mst_box_bobbin_cost WHERE bbc_code = v.code AND deleted_at IS NULL
);

COMMIT;
