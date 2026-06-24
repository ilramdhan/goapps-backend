DELETE FROM formula_param
WHERE formula_id IN (
    SELECT id FROM mst_formula WHERE created_by = 'seed_formula_oracle'
);
DELETE FROM mst_formula WHERE created_by = 'seed_formula_oracle';
