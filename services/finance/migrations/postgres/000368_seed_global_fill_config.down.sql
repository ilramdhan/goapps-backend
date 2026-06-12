-- Migration 000368 down: Remove seeded global fill config levels

DELETE FROM cost_level_assignment_config
WHERE clac_route_level IN (1, 2, 3, 4, 100, 101, 102)
  AND clac_created_by = 'system_seed';
