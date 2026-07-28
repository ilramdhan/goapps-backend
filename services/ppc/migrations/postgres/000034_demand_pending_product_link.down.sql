-- Revert 000034: restore the NOT NULL product on production_demand.
--
-- Decision: DOCUMENTED FAILURE, not backfill. Unlike 000033 (where the anchor
-- could be recovered from wo_plan_item_link), there is no recoverable value for
-- an unlinked demand's product — every candidate is an invented id, which is
-- exactly the corruption this feature exists to prevent.
--
-- So: if any demand is still PENDING_PRODUCT_LINK, the ALTER below fails loudly.
-- The operator must first link those demands (MapDemandProduct) or delete them,
-- then re-run the down migration. The SELECT immediately below names them.

DO $$
DECLARE
  unlinked BIGINT;
BEGIN
  SELECT count(*) INTO unlinked
  FROM production_demand
  WHERE pd_cpm_product_sys_id IS NULL;

  IF unlinked > 0 THEN
    RAISE EXCEPTION
      'cannot revert 000034: % demand(s) still have no linked product. Link them via MapDemandProduct or delete them, then retry. Offending ids: %',
      unlinked,
      (SELECT array_agg(pd_id ORDER BY pd_id) FROM production_demand WHERE pd_cpm_product_sys_id IS NULL);
  END IF;
END
$$;

ALTER TABLE production_demand DROP CONSTRAINT IF EXISTS chk_pd_product_link;
ALTER TABLE production_demand DROP CONSTRAINT IF EXISTS chk_pd_link_reason;

ALTER TABLE production_demand DROP COLUMN IF EXISTS pd_product_link_reason;

ALTER TABLE production_demand ALTER COLUMN pd_cpm_product_sys_id SET NOT NULL;

COMMENT ON COLUMN production_demand.pd_cpm_product_sys_id IS NULL;
