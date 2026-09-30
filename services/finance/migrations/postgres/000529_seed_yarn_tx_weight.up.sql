-- 000529 (backlog1 Track A) — seed TX Weight rules (design Item 1 seed table).
--
-- Product types are resolved by cost_product_type.cpt_type_code. Legacy
-- "TWISTED YARN" maps to TTY. A type code absent from cost_product_type is
-- skipped with a NOTICE (its products keep the ratio fallback).
--
-- ITY uses the user-decided formula (same as ACY: AE -0.5, A9 x0.65, A x0.35,
-- B =1, C =0.7), NOT the legacy ITY values (user decision 2026-09-29).
--
-- ytw_oracle_sys_id carries legacy CYTW_SYS_ID 20201001..20201025. The legacy
-- export gives no per-row mapping, so the ids are assigned in the legacy
-- screen order (POY, DTY, TWISTED YARN, ACY, ITY) x (AE, A9, A, B, C); they are
-- traceability only and never read by the engine.
--
-- Idempotent: a (type, grade) that already has a live row is left untouched.
BEGIN;

DO $$
DECLARE
    v_code     TEXT;
    v_inserted INT;
BEGIN
    FOREACH v_code IN ARRAY ARRAY['POY', 'DTY', 'TTY', 'ACY', 'ITY'] LOOP
        IF NOT EXISTS (SELECT 1 FROM cost_product_type WHERE cpt_type_code = v_code) THEN
            RAISE NOTICE '000529: product type % not found in cost_product_type — TX Weight rows skipped (ratio fallback applies).', v_code;
        END IF;
    END LOOP;

    INSERT INTO mst_yarn_tx_weight (
        ytw_product_type_id, ytw_grade, ytw_mode, ytw_value, ytw_description, ytw_oracle_sys_id, created_by
    )
    SELECT pt.cpt_type_id, v.grade, v.mode, v.value, v.descr, v.sys_id, 'seed_000529'
    FROM (VALUES
        ('POY', 'AE', 'LESS_BY',  8.0,   'POY AE: AX - 8',          '20201001'),
        ('POY', 'A9', 'LESS_BY',  0.0,   'POY A9: AX - 0',          '20201002'),
        ('POY', 'A',  'LESS_BY',  0.0,   'POY A: AX - 0',           '20201003'),
        ('POY', 'B',  'LESS_BY',  12.5,  'POY B: AX - 12.5',        '20201004'),
        ('POY', 'C',  'LESS_BY',  12.5,  'POY C: AX - 12.5',        '20201005'),
        ('DTY', 'AE', 'LESS_BY',  0.5,   'DTY AE: AX - 0.5',        '20201006'),
        ('DTY', 'A9', 'MULTIPLY', 0.65,  'DTY A9: AX x 0.65',       '20201007'),
        ('DTY', 'A',  'MULTIPLY', 0.35,  'DTY A: AX x 0.35',        '20201008'),
        ('DTY', 'B',  'FIXED',    2.5,   'DTY B: 2.5',              '20201009'),
        ('DTY', 'C',  'FIXED',    0.7,   'DTY C: 0.7',              '20201010'),
        ('TTY', 'AE', 'LESS_BY',  0.5,   'Twisted Yarn AE: AX - 0.5',  '20201011'),
        ('TTY', 'A9', 'MULTIPLY', 0.7,   'Twisted Yarn A9: AX x 0.7',  '20201012'),
        ('TTY', 'A',  'MULTIPLY', 0.45,  'Twisted Yarn A: AX x 0.45',  '20201013'),
        ('TTY', 'B',  'FIXED',    1.0,   'Twisted Yarn B: 1',          '20201014'),
        ('TTY', 'C',  'FIXED',    0.7,   'Twisted Yarn C: 0.7',        '20201015'),
        ('ACY', 'AE', 'LESS_BY',  0.5,   'ACY AE: AX - 0.5',        '20201016'),
        ('ACY', 'A9', 'MULTIPLY', 0.65,  'ACY A9: AX x 0.65',       '20201017'),
        ('ACY', 'A',  'MULTIPLY', 0.35,  'ACY A: AX x 0.35',        '20201018'),
        ('ACY', 'B',  'FIXED',    1.0,   'ACY B: 1',                '20201019'),
        ('ACY', 'C',  'FIXED',    0.7,   'ACY C: 0.7',              '20201020'),
        ('ITY', 'AE', 'LESS_BY',  0.5,   'ITY AE: AX - 0.5',        '20201021'),
        ('ITY', 'A9', 'MULTIPLY', 0.65,  'ITY A9: AX x 0.65',       '20201022'),
        ('ITY', 'A',  'MULTIPLY', 0.35,  'ITY A: AX x 0.35',        '20201023'),
        ('ITY', 'B',  'FIXED',    1.0,   'ITY B: 1',                '20201024'),
        ('ITY', 'C',  'FIXED',    0.7,   'ITY C: 0.7',              '20201025')
    ) AS v(type_code, grade, mode, value, descr, sys_id)
    JOIN cost_product_type pt ON pt.cpt_type_code = v.type_code
    WHERE NOT EXISTS (
        SELECT 1 FROM mst_yarn_tx_weight w
        WHERE w.ytw_product_type_id = pt.cpt_type_id
          AND w.ytw_grade = v.grade
          AND w.deleted_at IS NULL
    );

    GET DIAGNOSTICS v_inserted = ROW_COUNT;
    RAISE NOTICE '000529: inserted % TX Weight row(s).', v_inserted;
END $$;

COMMIT;
