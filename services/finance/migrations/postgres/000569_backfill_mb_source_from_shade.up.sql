-- 000569 — Backfill EMPTY MB_SP_CODE / MB_SP_DYE (+ spin fill-group children) from the product shade.
--
-- Mirrors the runtime auto-fill (internal/application/mbsourceautofill + domain/mbsource) for the
-- products that existed before the hook shipped. Rules (user-approved 2026-10-10):
--   * Shade match = UPPER(TRIM()) on both sides, exact.
--   * Eligible product: not locked, not MB-typed, shade set, MB_SP_CODE has NO value yet
--     (no row or empty text). Products holding any MB_SP_CODE value are never touched.
--   * Source 1 (wins): mst_mb_spin, live + active. Several rows per shade -> ONE deterministic pick:
--     Spinning > Boughtout > R and D (anything else last), then newest, then id (same ORDER BY text as
--     mbsource.SpinPickOrderSQL; a Go test asserts the two stay identical).
--       MB_SP_CODE = orion item code (else mb_costing, else spin id) + cpp_value_mb_spin_id = spin id,
--       children = the spin fill-group columns (same mapping as the Param-tab fill).
--   * Source 2: cost_superba_cost_sp (active, max legacy_sys_id per shade):
--       MB_SP_CODE = shade (normalized), MB_SP_DYE = colour name; rate/dozing/other children left empty.
--   * Only EMPTY cells are written; CAPP rows (MB_SP_CODE + children) are attached ONLY for products
--     whose shade resolved. No recalculation is performed.
--
-- Reversible: bak_000569_values (touched value rows, had_row=FALSE for inserts) and bak_000569_capp
-- (CAPP rows inserted here). Marker: cpp_filled_by / created_by = 'backfill_mb_source_000569'.
-- Idempotent: a second run finds the products filled and changes nothing.
BEGIN;

CREATE TABLE IF NOT EXISTS bak_000569_values (
    product_sys_id    BIGINT       NOT NULL,
    param_id          UUID         NOT NULL,
    param_code        VARCHAR(100) NOT NULL,
    had_row           BOOLEAN      NOT NULL,
    old_value_numeric NUMERIC(20,6),
    old_value_text    TEXT,
    old_value_flag    BOOLEAN,
    old_mb_spin_id    UUID,
    old_filled_at     TIMESTAMPTZ,
    old_filled_by     VARCHAR(100),
    old_updated_at    TIMESTAMPTZ,
    old_updated_by    VARCHAR(100),
    backed_up_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (product_sys_id, param_id)
);
CREATE TABLE IF NOT EXISTS bak_000569_capp (
    product_sys_id BIGINT      NOT NULL,
    param_id       UUID        NOT NULL,
    backed_up_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (product_sys_id, param_id)
);
COMMENT ON TABLE bak_000569_values IS 'Backup written by migration 000569; read by its down migration. Safe to drop once 000569 is final.';
COMMENT ON TABLE bak_000569_capp   IS 'CAPP rows inserted by migration 000569; read by its down migration. Safe to drop once 000569 is final.';

-- Eligible products (shade set, unlocked, not MB, MB_SP_CODE without a value).
CREATE TEMP TABLE tmp_000569_prod ON COMMIT DROP AS
SELECT m.cpm_product_sys_id AS product_sys_id, UPPER(TRIM(m.cpm_shade_code)) AS shade
FROM cost_product_master m
JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
WHERE NOT m.cpm_is_locked
  AND t.cpt_type_code <> 'MB'
  AND COALESCE(TRIM(m.cpm_shade_code), '') <> ''
  AND NOT EXISTS (
        SELECT 1 FROM cost_product_parameter c
        JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = 'MB_SP_CODE' AND p.deleted_at IS NULL
        WHERE c.cpp_product_sys_id = m.cpm_product_sys_id AND COALESCE(c.cpp_value_text, '') <> '');

-- Source 1: one spin per shade, ordered by the shared pick rule.
CREATE TEMP TABLE tmp_000569_spin ON COMMIT DROP AS
SELECT DISTINCT ON (UPPER(TRIM(mbs_shade_code))) UPPER(TRIM(mbs_shade_code)) AS shade, s.*
FROM mst_mb_spin s
WHERE deleted_at IS NULL AND mbs_is_active
  AND COALESCE(TRIM(mbs_shade_code), '') <> ''
  AND UPPER(TRIM(mbs_shade_code)) IN (SELECT shade FROM tmp_000569_prod)
ORDER BY UPPER(TRIM(mbs_shade_code)),
	CASE mbs_status WHEN 'Spinning' THEN 1 WHEN 'Boughtout' THEN 2 WHEN 'R and D' THEN 3 ELSE 4 END,
	GREATEST(created_at, COALESCE(updated_at, created_at)) DESC,
	mbs_id;

-- Source 2: superba (active, max legacy_sys_id per shade).
CREATE TEMP TABLE tmp_000569_sup ON COMMIT DROP AS
SELECT DISTINCT ON (UPPER(TRIM(shade_code))) UPPER(TRIM(shade_code)) AS shade, COALESCE(colour_name, '') AS colour
FROM cost_superba_cost_sp
WHERE is_active AND deleted_at IS NULL
  AND UPPER(TRIM(shade_code)) IN (SELECT shade FROM tmp_000569_prod)
ORDER BY UPPER(TRIM(shade_code)), legacy_sys_id DESC;

-- Candidate values per (product, param); spin wins over superba.
CREATE TEMP TABLE tmp_000569_cand ON COMMIT DROP AS
SELECT p.product_sys_id, 'MB_SP_CODE'::text AS param_code, NULL::numeric AS num,
       COALESCE(NULLIF(s.mbs_orion_item_code, ''), NULLIF(s.mbs_mb_costing, ''), s.mbs_id::text) AS txt,
       s.mbs_id AS spin_id, 'MB_SPIN'::text AS source
FROM tmp_000569_prod p JOIN tmp_000569_spin s ON s.shade = p.shade
UNION ALL
SELECT p.product_sys_id, ch.param_code,
       CASE ch.lookup_source_column
            WHEN 'mbs_denier'        THEN s.mbs_denier
            WHEN 'mbs_dozing'        THEN s.mbs_dozing
            WHEN 'mbs_run_ldr_pct'   THEN s.mbs_run_ldr_pct
            WHEN 'mbs_ldr_prsn'      THEN s.mbs_ldr_prsn
            WHEN 'mbs_filament'      THEN s.mbs_filament::numeric
            WHEN 'mbs_cost_rate_mkt' THEN s.mbs_cost_rate_mkt
       END,
       CASE ch.lookup_source_column
            WHEN 'mbs_mgt_name' THEN NULLIF(s.mbs_mgt_name, '')
            WHEN 'mbs_cc'       THEN NULLIF(s.mbs_cc, '')
       END,
       NULL::uuid, 'MB_SPIN'
FROM tmp_000569_prod p
JOIN tmp_000569_spin s ON s.shade = p.shade
JOIN mst_parameter ch ON ch.lookup_fill_group_code = 'MB_SP_CODE' AND ch.deleted_at IS NULL
UNION ALL
SELECT p.product_sys_id, 'MB_SP_CODE', NULL, u.shade, NULL::uuid, 'SUPERBA_COST_SP'
FROM tmp_000569_prod p JOIN tmp_000569_sup u ON u.shade = p.shade
WHERE NOT EXISTS (SELECT 1 FROM tmp_000569_spin s WHERE s.shade = p.shade)
UNION ALL
SELECT p.product_sys_id, 'MB_SP_DYE', NULL, u.colour, NULL::uuid, 'SUPERBA_COST_SP'
FROM tmp_000569_prod p JOIN tmp_000569_sup u ON u.shade = p.shade
WHERE u.colour <> '' AND NOT EXISTS (SELECT 1 FROM tmp_000569_spin s WHERE s.shade = p.shade);

DELETE FROM tmp_000569_cand WHERE num IS NULL AND COALESCE(txt, '') = '';

-- Resolve param ids; keep only cells that are missing or empty (never overwrite).
CREATE TEMP TABLE tmp_000569_target ON COMMIT DROP AS
SELECT c.product_sys_id, mp.id AS param_id, c.param_code, c.num, c.txt, c.spin_id, c.source
FROM tmp_000569_cand c
JOIN mst_parameter mp ON mp.param_code = c.param_code AND mp.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM cost_product_parameter x
    WHERE x.cpp_product_sys_id = c.product_sys_id AND x.cpp_param_id = mp.id
      AND (x.cpp_value_numeric IS NOT NULL OR x.cpp_value_flag IS NOT NULL OR COALESCE(x.cpp_value_text, '') <> ''));

DO $$
DECLARE n_elig BIGINT; n_spin BIGINT; n_sup BIGINT; n_amb BIGINT; n_nomatch BIGINT; n_filled BIGINT;
BEGIN
    SELECT COUNT(*) INTO n_elig FROM tmp_000569_prod;
    SELECT COUNT(*) INTO n_spin FROM tmp_000569_target WHERE param_code = 'MB_SP_CODE' AND source = 'MB_SPIN';
    SELECT COUNT(*) INTO n_sup  FROM tmp_000569_target WHERE param_code = 'MB_SP_CODE' AND source = 'SUPERBA_COST_SP';
    SELECT COUNT(*) INTO n_amb FROM tmp_000569_prod p
     WHERE (SELECT COUNT(*) FROM mst_mb_spin s WHERE s.deleted_at IS NULL AND s.mbs_is_active
              AND UPPER(TRIM(s.mbs_shade_code)) = p.shade) > 1;
    SELECT COUNT(*) INTO n_nomatch FROM tmp_000569_prod p
     WHERE NOT EXISTS (SELECT 1 FROM tmp_000569_spin s WHERE s.shade = p.shade)
       AND NOT EXISTS (SELECT 1 FROM tmp_000569_sup u WHERE u.shade = p.shade);
    SELECT COUNT(*) INTO n_filled FROM cost_product_master m
      JOIN cost_product_parameter c ON c.cpp_product_sys_id = m.cpm_product_sys_id
      JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = 'MB_SP_CODE' AND p.deleted_at IS NULL
     WHERE COALESCE(c.cpp_value_text, '') <> '' AND COALESCE(TRIM(m.cpm_shade_code), '') <> '';
    RAISE NOTICE '000569 eligible products (shade set, MB_SP_CODE empty): %', n_elig;
    RAISE NOTICE '000569 to fill from MB spin: %', n_spin;
    RAISE NOTICE '000569 to fill from Superba Cost SP: %', n_sup;
    RAISE NOTICE '000569 of eligible, shade matches >1 live spin (ambiguous, deterministic pick): %', n_amb;
    RAISE NOTICE '000569 skipped no match: %', n_nomatch;
    RAISE NOTICE '000569 skipped already filled (shade set, MB_SP_CODE has value): %', n_filled;
END $$;

-- PART 1: back up what will change (existing-but-empty rows, and brand-new inserts).
INSERT INTO bak_000569_values (product_sys_id, param_id, param_code, had_row,
    old_value_numeric, old_value_text, old_value_flag, old_mb_spin_id,
    old_filled_at, old_filled_by, old_updated_at, old_updated_by)
SELECT t.product_sys_id, t.param_id, t.param_code, (x.cpp_value_id IS NOT NULL),
       x.cpp_value_numeric, x.cpp_value_text, x.cpp_value_flag, x.cpp_value_mb_spin_id,
       x.cpp_filled_at, x.cpp_filled_by, x.cpp_updated_at, x.cpp_updated_by
FROM tmp_000569_target t
LEFT JOIN cost_product_parameter x ON x.cpp_product_sys_id = t.product_sys_id AND x.cpp_param_id = t.param_id
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

-- PART 2: attach CAPP (trigger + children) for products that resolved.
INSERT INTO bak_000569_capp (product_sys_id, param_id)
SELECT DISTINCT r.product_sys_id, mp.id
FROM (SELECT DISTINCT product_sys_id FROM tmp_000569_target) r
JOIN mst_parameter mp ON mp.deleted_at IS NULL
                     AND (mp.param_code = 'MB_SP_CODE' OR mp.lookup_fill_group_code = 'MB_SP_CODE')
WHERE NOT EXISTS (SELECT 1 FROM cost_product_applicable_param c
                  WHERE c.capp_product_sys_id = r.product_sys_id AND c.capp_param_id = mp.id)
ON CONFLICT (product_sys_id, param_id) DO NOTHING;

INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
SELECT b.product_sys_id, b.param_id, FALSE, 'backfill_mb_source_000569' FROM bak_000569_capp b
ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING;

-- PART 3: write values (empty cells only).
DO $$
DECLARE n BIGINT;
BEGIN
    INSERT INTO cost_product_parameter (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_value_text,
                                        cpp_value_mb_spin_id, cpp_filled_by, cpp_created_by)
    SELECT t.product_sys_id, t.param_id, t.num, CASE WHEN t.num IS NULL THEN t.txt END, t.spin_id,
           'backfill_mb_source_000569', 'backfill_mb_source_000569'
    FROM tmp_000569_target t
    ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET
        cpp_value_numeric = EXCLUDED.cpp_value_numeric, cpp_value_text = EXCLUDED.cpp_value_text,
        cpp_value_mb_spin_id = EXCLUDED.cpp_value_mb_spin_id,
        cpp_filled_at = NOW(), cpp_filled_by = EXCLUDED.cpp_filled_by,
        cpp_updated_at = NOW(), cpp_updated_by = EXCLUDED.cpp_filled_by
    WHERE cost_product_parameter.cpp_value_numeric IS NULL AND cost_product_parameter.cpp_value_flag IS NULL
      AND COALESCE(cost_product_parameter.cpp_value_text, '') = '';
    GET DIAGNOSTICS n = ROW_COUNT;
    RAISE NOTICE '000569 value rows written: %', n;
END $$;

COMMIT;
