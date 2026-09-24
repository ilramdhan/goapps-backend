-- 000524 down — restore the 000408 oil formulas and remove what 000524 added.
--
-- Order matters (000510 pattern): edges are deleted FIRST, while the
-- 'oil_by_type_000524' marker still identifies the formulas this migration
-- rewrote. Only the edges 000524 added are removed; F_YARN_OIL_COST keeps its
-- original OIL_RATE / OPU edges from 000408.
-- updated_at / updated_by / description go back to what 000408 seeded (its
-- INSERT set no updated_* columns).

BEGIN;

DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id
  AND fp.param_id = p.id
  AND f.deleted_at IS NULL
  AND f.updated_by = 'oil_by_type_000524'
  AND (
        (f.formula_code = 'F_YARN_OIL_COST' AND p.param_code = 'WASTE_PERC')
     OR (f.formula_code = 'F_YARN_OIL_GAIN' AND p.param_code IN ('OPU', 'OIL_RATE', 'OIL_GAIN_POY_DEFAULT'))
  );

UPDATE mst_formula
SET expression  = 'OIL_RATE * OPU / 100.0',
    description = 'Oil cost per kg',
    updated_at  = NULL,
    updated_by  = NULL
WHERE formula_code = 'F_YARN_OIL_COST'
  AND deleted_at IS NULL
  AND updated_by = 'oil_by_type_000524'
  AND expression = '(IS_PTY == 1 || IS_POY == 1) ? ((OPU / (1 - WASTE_PERC / 100)) / (1 - 0.11)) * OIL_RATE / 100 : (IS_SUPERBA == 1 ? (OIL_RATE * OPU) / 1000 : 0)';

UPDATE mst_formula
SET expression  = '0',
    description = 'Always 0',
    updated_at  = NULL,
    updated_by  = NULL
WHERE formula_code = 'F_YARN_OIL_GAIN'
  AND deleted_at IS NULL
  AND updated_by = 'oil_by_type_000524'
  AND expression = 'IS_PTY == 1 ? ((OPU * OIL_RATE) / 100) * -1 : (IS_POY == 1 ? OIL_GAIN_POY_DEFAULT : 0)';

-- Soft-delete the CONSTANT formula (only the row this migration created) and
-- drop any edges pointing at it (CONSTANT has none by construction).
DELETE FROM formula_param fp
USING mst_formula f
WHERE fp.formula_id = f.id
  AND f.formula_code = 'F_YARN_OIL_GAIN_POY_DEFAULT'
  AND f.created_by = 'seed_000524'
  AND f.deleted_at IS NULL;

UPDATE mst_formula
SET deleted_at = NOW(),
    deleted_by = 'migration_000524_down'
WHERE formula_code = 'F_YARN_OIL_GAIN_POY_DEFAULT'
  AND created_by = 'seed_000524'
  AND deleted_at IS NULL;

COMMIT;
