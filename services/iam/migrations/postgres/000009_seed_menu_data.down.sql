-- Rollback: Remove seeded menu data
-- Only removes records created by the seed script (created_by = 'seed')

DELETE FROM menu_permissions
WHERE menu_id IN (
    SELECT menu_id FROM mst_menu WHERE created_by = 'seed'
);

-- Delete in reverse dependency order (children before parents)
DELETE FROM mst_menu WHERE created_by = 'seed' AND menu_level = 3;
DELETE FROM mst_menu WHERE created_by = 'seed' AND menu_level = 2;
DELETE FROM mst_menu WHERE created_by = 'seed' AND menu_level = 1;
