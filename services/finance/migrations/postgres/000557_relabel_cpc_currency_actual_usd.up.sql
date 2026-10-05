-- 000557 — relabel ACTUAL product-cost rows IDR -> USD, per period, guarded
-- (design Part 1 §4.9, §7.3; plan-01 P0-T10b; sql-1-findings F-3/F-4/F-10;
-- User decision 2026-09-29 U-3).
--
-- LABEL ONLY. cpc_cost_per_unit and every other value column are untouched;
-- the ACTUAL engine has always computed USD and only the label was wrong.
--
-- Numbering: labelled 000557; 000556 is intentionally absent (gated
-- migration, reserved). golang-migrate tolerates gaps (see the existing
-- 000025->000100, 000510->000512 gaps). Renumber to the next free number at
-- merge time if needed.
--
-- Scope: an EXPLICIT period list — 202604, 202606, 202607, 202608, 202609.
-- 202605 is EXCLUDED (broken early run, 9,704 rows > 50, max 19,271.5; F-4,
-- F-10). A later migration relabels 202605 only after E-11.
--
-- Guard (per period, before anything is written): count active ACTUAL rows
-- (cpc_status <> 'SUPERSEDED') with cpc_cost_per_unit > the sanity threshold.
-- If any period has one, RAISE — the whole file is one transaction, so
-- nothing is changed. Threshold 100 (observed legitimate maxima ~92).
--
-- Every relabelled row is logged in cst_currency_relabel_log; the down
-- migration restores exactly those rows. Runs as a no-op on an empty table.
-- Never auto-applied: the user applies migrations.

BEGIN;

CREATE TABLE IF NOT EXISTS cst_currency_relabel_log (
    ccrl_id             BIGSERIAL   PRIMARY KEY,
    ccrl_cpc_cost_id    BIGINT      NOT NULL,
    ccrl_product_sys_id BIGINT      NOT NULL,
    ccrl_period         VARCHAR(6)  NOT NULL,
    ccrl_old_label      VARCHAR(3)  NOT NULL,
    ccrl_new_label      VARCHAR(3)  NOT NULL,
    ccrl_migration      VARCHAR(40) NOT NULL,
    ccrl_relabelled_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uk_ccrl_cost_migration UNIQUE (ccrl_cpc_cost_id, ccrl_migration)
);

COMMENT ON TABLE cst_currency_relabel_log IS
    'Rows whose cpc_currency_code label was changed by a relabel migration (design §4.9). Down migrations restore from here, touched rows only.';

DO $$
DECLARE
    c_threshold CONSTANT NUMERIC := 100;
    c_marker    CONSTANT TEXT    := 'migration:000557';
    v_periods   CONSTANT TEXT[]  := ARRAY['202604', '202606', '202607', '202608', '202609'];
    v_period    TEXT;
    v_outliers  BIGINT;
    v_logged    BIGINT;
    v_updated   BIGINT;
BEGIN
    -- 1. Guard every period first, so a failure changes nothing anywhere.
    FOREACH v_period IN ARRAY v_periods LOOP
        SELECT count(*) INTO v_outliers
          FROM cst_product_cost
         WHERE cpc_calculation_type = 'ACTUAL'
           AND cpc_status <> 'SUPERSEDED'
           AND cpc_period = v_period
           AND cpc_cost_per_unit > c_threshold;
        IF v_outliers > 0 THEN
            RAISE EXCEPTION '000557 guard: period % has % active ACTUAL rows with cost_per_unit > % — relabel aborted, nothing changed',
                v_period, v_outliers, c_threshold;
        END IF;
    END LOOP;

    -- 2. Log then relabel, per period.
    FOREACH v_period IN ARRAY v_periods LOOP
        INSERT INTO cst_currency_relabel_log
               (ccrl_cpc_cost_id, ccrl_product_sys_id, ccrl_period, ccrl_old_label, ccrl_new_label, ccrl_migration)
        SELECT cpc_cost_id, cpc_product_sys_id, cpc_period, cpc_currency_code, 'USD', c_marker
          FROM cst_product_cost
         WHERE cpc_calculation_type = 'ACTUAL'
           AND cpc_currency_code = 'IDR'
           AND cpc_period = v_period
        ON CONFLICT (ccrl_cpc_cost_id, ccrl_migration) DO NOTHING;
        GET DIAGNOSTICS v_logged = ROW_COUNT;

        UPDATE cst_product_cost
           SET cpc_currency_code = 'USD'
         WHERE cpc_calculation_type = 'ACTUAL'
           AND cpc_currency_code = 'IDR'
           AND cpc_period = v_period;
        GET DIAGNOSTICS v_updated = ROW_COUNT;

        IF v_updated <> v_logged THEN
            RAISE EXCEPTION '000557: period % logged % rows but relabelled % — aborted', v_period, v_logged, v_updated;
        END IF;

        RAISE NOTICE '000557: period % relabelled % ACTUAL rows IDR -> USD', v_period, v_updated;
    END LOOP;
END
$$;

COMMIT;
