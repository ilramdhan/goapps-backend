ALTER TABLE mst_formula
    DROP CONSTRAINT IF EXISTS mst_formula_formula_type_check;

ALTER TABLE mst_formula
    ADD CONSTRAINT mst_formula_formula_type_check
        CHECK (formula_type IN (
            'CALCULATION', 'SQL_QUERY', 'CONSTANT',
            'LOOKUP', 'RM_LOOKUP', 'CONDITIONAL',
            'FROM_MARKETING', 'INTERMINGLING', 'SNAPSHOT',
            'PENDING', 'INITIAL_VALUE'
        ));
