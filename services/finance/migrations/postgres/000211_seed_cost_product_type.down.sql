-- Reverting the seed is unsafe (would orphan downstream cost_product_master rows
-- that point to these type ids). Down migration is a no-op; restore via fresh
-- container if seed needs to be removed entirely.
SELECT 1;
