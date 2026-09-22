-- 000518_configure_yarn_rm_rate_cascade.up.sql
--
-- Makes the GROUP-type RM cost cascade order (currently hardcoded CR->SR->PR in
-- resolveRMUnitCost, services/finance/internal/application/costcalc/compute.go)
-- data-driven via F_YARN_RM_RATE's expression column, without building a full
-- DSL/expression interpreter for the legacy Oracle expression this row has
-- carried since 000408_seed_oracle_formulas.up.sql.
--
-- F_YARN_RM_RATE's formula_type is RM_LOOKUP, which is never evaluated by the
-- expr-lang evaluator (see compute.go's FormulaType switch) -- its expression
-- column has been dead/display-only text since the calc engine was written, so
-- repurposing it as a small config value changes no runtime behavior on its own.
--
-- New format contract: a comma-separated, ordered list of rate codes, valid
-- tokens CR/SR/PR, read by costcalc.ParseRMRateOrder (loader.go) as the
-- GROUP-RM fallback priority order (first non-zero wins). Unparseable/invalid
-- content falls back to the historical CR,SR,PR order -- never hard-fails a
-- cost computation over a config typo.
UPDATE mst_formula
SET expression = 'CR,SR,PR',
    updated_at = NOW(),
    updated_by = 'migration_000518'
WHERE formula_code = 'F_YARN_RM_RATE';
