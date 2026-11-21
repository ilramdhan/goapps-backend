-- 000614: per-machine fixed initial value for CALCULATED yarn params.
--
-- Finance feedback: for some machines a CALCULATED param is a fixed number,
-- not its formula:
--   MB_COST_MKT    (F_YARN_MB_COST)    SP Dyeing 0.08, Hanking (Cone to Hank) 0.02
--   STEAM_COST_CNG (F_YARN_STEAM_COST) Washing 0.019
-- Every other machine keeps the formula. The engine (costcalc
-- applyMachineDefaults) skips the formula for a product whose MC_NAME matches a
-- configured machine and uses the product's stored value
-- (cost_product_parameter.cpp_value_numeric, editable per product) or, when
-- none, the machine default below.
--
-- PART 1: table mst_machine_param_default (schema).
-- PART 2: seed the 3 rows (skipped silently when the machine/param is missing).
-- PART 3: backfill products whose MC_NAME matches a configured machine:
--         attach the param (CAPP) and store the default ONLY when the product
--         has no value yet. Existing values (e.g. edited 0.1 / 0.0191) are kept.
-- Marker 'machine_param_default_000614' (created_by / capp_created_by /
-- cpp_filled_by); the down migration deletes by that marker only.
-- Machine match (same as the engine loader): trimmed, case-insensitive MC_NAME
-- text against mst_machine.mc_name OR mc_code.

BEGIN;

-- PART 1 ---------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mst_machine_param_default (
    mmpd_id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    mmpd_machine_id    UUID          NOT NULL REFERENCES mst_machine (mc_id),
    mmpd_param_id      UUID          NOT NULL REFERENCES mst_parameter (id),
    mmpd_default_value NUMERIC(20,6) NOT NULL,
    mmpd_notes         TEXT,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by         VARCHAR(100)  NOT NULL,
    updated_at         TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    deleted_at         TIMESTAMPTZ,
    deleted_by         VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mmpd_machine_param
    ON mst_machine_param_default (mmpd_machine_id, mmpd_param_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mmpd_param
    ON mst_machine_param_default (mmpd_param_id)
    WHERE deleted_at IS NULL;

-- PART 2 ---------------------------------------------------------------------
INSERT INTO mst_machine_param_default (
    mmpd_machine_id, mmpd_param_id, mmpd_default_value, mmpd_notes, created_by
)
SELECT DISTINCT ON (m.mc_id, p.id)
       m.mc_id, p.id, v.val, v.notes, 'machine_param_default_000614'
FROM (VALUES
    ('SP Dyeing',              'MB_COST_MKT',    0.08,  'Finance: fixed MB cost for SP Dyeing'),
    ('Hanking (Cone to Hank)', 'MB_COST_MKT',    0.02,  'Finance: fixed MB cost for Hanking'),
    ('Washing',                'STEAM_COST_CNG', 0.019, 'Finance: fixed steam cost for Washing')
) AS v(machine, param_code, val, notes)
JOIN mst_machine m
  ON m.deleted_at IS NULL
 AND (LOWER(BTRIM(m.mc_name)) = LOWER(v.machine) OR LOWER(BTRIM(m.mc_code)) = LOWER(v.machine))
JOIN mst_parameter p
  ON p.deleted_at IS NULL
 AND p.param_code = v.param_code
WHERE NOT EXISTS (
    SELECT 1 FROM mst_machine_param_default d
    WHERE d.mmpd_machine_id = m.mc_id
      AND d.mmpd_param_id = p.id
      AND d.deleted_at IS NULL
)
ORDER BY m.mc_id, p.id;

-- PART 3 (data backfill) -----------------------------------------------------
-- Targets: live, active products whose MC_NAME matches a machine that has a
-- live default row. One row per (product, param).
CREATE TEMP TABLE tmp_mmpd_000614_target ON COMMIT DROP AS
SELECT DISTINCT ON (cpp.cpp_product_sys_id, d.mmpd_param_id)
       cpp.cpp_product_sys_id AS product_sys_id,
       d.mmpd_param_id        AS param_id,
       d.mmpd_default_value   AS default_value
FROM cost_product_parameter cpp
JOIN mst_parameter mc
  ON mc.id = cpp.cpp_param_id
 AND mc.param_code = 'MC_NAME'
 AND mc.deleted_at IS NULL
JOIN cost_product_master pm
  ON pm.cpm_product_sys_id = cpp.cpp_product_sys_id
 AND pm.cpm_is_active
JOIN mst_machine m
  ON m.deleted_at IS NULL
 AND (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))
      OR LOWER(BTRIM(m.mc_code)) = LOWER(BTRIM(cpp.cpp_value_text)))
JOIN mst_machine_param_default d
  ON d.mmpd_machine_id = m.mc_id
 AND d.deleted_at IS NULL
WHERE NULLIF(BTRIM(cpp.cpp_value_text), '') IS NOT NULL
ORDER BY cpp.cpp_product_sys_id, d.mmpd_param_id,
         (LOWER(BTRIM(m.mc_name)) = LOWER(BTRIM(cpp.cpp_value_text))) DESC;

-- (a) attach the param where missing.
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
)
SELECT t.product_sys_id, t.param_id, FALSE, NULL::INT, 'machine_param_default_000614'
FROM tmp_mmpd_000614_target t
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

-- (b) store the default only where the product has no row yet (keep edits).
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_filled_by, cpp_created_by
)
SELECT t.product_sys_id, t.param_id, t.default_value,
       'machine_param_default_000614', 'machine_param_default_000614'
FROM tmp_mmpd_000614_target t
WHERE NOT EXISTS (
    SELECT 1 FROM cost_product_parameter c
    WHERE c.cpp_product_sys_id = t.product_sys_id
      AND c.cpp_param_id = t.param_id
)
ON CONFLICT ON CONSTRAINT cpp_unique_product_param DO NOTHING;

COMMIT;
