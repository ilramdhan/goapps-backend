-- 000530 down: restore the exact 000408 grade-weight expressions, only on rows
-- this migration rewired (marker updated_by = 'tx_weight_000530').

BEGIN;

UPDATE mst_formula
SET expression = 'AX_WT * AE_PERC / AX_PERC',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_AE_WT'
  AND deleted_at IS NULL
  AND updated_by = 'tx_weight_000530'
  AND expression = 'tx_weight(''AE'', AX_WT, AX_WT * AE_PERC / AX_PERC)';

UPDATE mst_formula
SET expression = 'AX_WT * A9_PERC / AX_PERC',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_A9_WT'
  AND deleted_at IS NULL
  AND updated_by = 'tx_weight_000530'
  AND expression = 'tx_weight(''A9'', AX_WT, AX_WT * A9_PERC / AX_PERC)';

UPDATE mst_formula
SET expression = 'AX_WT * A_PERC / AX_PERC',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_A_WT'
  AND deleted_at IS NULL
  AND updated_by = 'tx_weight_000530'
  AND expression = 'tx_weight(''A'', AX_WT, AX_WT * A_PERC / AX_PERC)';

UPDATE mst_formula
SET expression = 'AX_WT * B_PERC / AX_PERC',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_B_WT'
  AND deleted_at IS NULL
  AND updated_by = 'tx_weight_000530'
  AND expression = 'tx_weight(''B'', AX_WT, AX_WT * B_PERC / AX_PERC)';

UPDATE mst_formula
SET expression = 'AX_WT * C_PERC / AX_PERC',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_C_WT'
  AND deleted_at IS NULL
  AND updated_by = 'tx_weight_000530'
  AND expression = 'tx_weight(''C'', AX_WT, AX_WT * C_PERC / AX_PERC)';

COMMIT;
