-- 000525 (oil-cost-rm-group M6) — checklist the 7 oil params into CAPP.
--
-- Every product whose type has an oil class (cost_product_type.cpt_oil_class,
-- 000521 — PTY / POY / TCS / TPS / TTS) must carry these CAPP rows (U11):
--   OIL_NAME, OIL_RATE, OIL_COST, OIL_GAIN, OIL_GAIN_POY_DEFAULT, OPU, WASTE_PERC
-- LoadFormulas only runs a formula whose result param is in the product's CAPP,
-- so without OIL_GAIN_POY_DEFAULT the POY gain would silently be 0.
--
-- Scope is RELATIONAL, zero literal product_sys_id: products joined to a type
-- with cpt_oil_class, MB types excluded defensively (MB never has an oil class).
-- Marker 'seed_oil_capp_000525' in capp_created_by; the .down.sql deletes by
-- that marker only. Rows that already exist are left untouched (ON CONFLICT
-- DO NOTHING on capp_unique_product_param).
--
-- Products gaining OPU / WASTE_PERC CAPP rows with no value compute OIL_COST 0
-- (open item O-8 — user decides whether acceptable; quantified by PF-5).

BEGIN;

DO $$
DECLARE
    v_defined INT;
BEGIN
    SELECT COUNT(*) INTO v_defined
    FROM mst_parameter
    WHERE deleted_at IS NULL
      AND param_code IN ('OIL_NAME', 'OIL_RATE', 'OIL_COST', 'OIL_GAIN',
                         'OIL_GAIN_POY_DEFAULT', 'OPU', 'WASTE_PERC');
    RAISE NOTICE '000525 pre-flight: oil params defined = % / 7', v_defined;
    IF v_defined < 7 THEN
        RAISE NOTICE '000525: some oil params are missing — rows are inserted only for the params that exist (expected on a migration-only DB only if 000407 was skipped).';
    END IF;
END $$;

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
)
SELECT pm.cpm_product_sys_id, mp.id, FALSE, NULL::INT, 'seed_oil_capp_000525'
FROM cost_product_master pm
JOIN cost_product_type pt
  ON pt.cpt_type_id = pm.cpm_product_type_id
 AND pt.cpt_oil_class IS NOT NULL
 AND pt.cpt_type_code <> 'MB'
CROSS JOIN mst_parameter mp
WHERE mp.deleted_at IS NULL
  AND mp.param_code IN ('OIL_NAME', 'OIL_RATE', 'OIL_COST', 'OIL_GAIN',
                        'OIL_GAIN_POY_DEFAULT', 'OPU', 'WASTE_PERC')
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

DO $$
DECLARE
    r RECORD;
    v_total BIGINT := 0;
BEGIN
    FOR r IN
        SELECT mp.param_code, COUNT(*) AS n
        FROM cost_product_applicable_param c
        JOIN mst_parameter mp ON mp.id = c.capp_param_id
        WHERE c.capp_created_by = 'seed_oil_capp_000525'
        GROUP BY mp.param_code
        ORDER BY mp.param_code
    LOOP
        RAISE NOTICE '000525: CAPP rows inserted for % = %', r.param_code, r.n;
        v_total := v_total + r.n;
    END LOOP;
    RAISE NOTICE '000525: CAPP rows inserted total = % (compare with PF-5)', v_total;
END $$;

COMMIT;
