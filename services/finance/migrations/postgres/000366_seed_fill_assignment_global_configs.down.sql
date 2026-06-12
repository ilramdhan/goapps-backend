-- 000365 down: remove the seeded global configs for levels 1-3.
-- Only deletes rows created by the seed (clac_created_by = 'system').
DELETE FROM cost_level_assignment_config
WHERE clac_created_by = 'system'
  AND clac_route_level IN (1, 2, 3);
