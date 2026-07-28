-- 000034: deferred product link for MTS / SAMPLE demands.
--
-- An MTS or SAMPLE demand is often raised before the finance cost-product
-- master row exists, so forcing a product at creation time pushed planners to
-- pick a stand-in id — which silently corrupts the product link. Allow the
-- product to be NULL, but only in the one status that means "not linked yet",
-- and record WHY it is unlinked so "intentionally unresolved" (NO_MASTER_YET)
-- stays distinguishable from "auto-match failed".

ALTER TABLE production_demand ALTER COLUMN pd_cpm_product_sys_id DROP NOT NULL;

ALTER TABLE production_demand
  ADD COLUMN IF NOT EXISTS pd_product_link_reason VARCHAR(20);

DO $$
BEGIN
  -- Biconditional on purpose: status and product-nullness must not drift apart.
  -- A NULL product in any other status is an unlinked demand masquerading as a
  -- plannable one; a PENDING_PRODUCT_LINK row with a product is a link that was
  -- never completed in the domain.
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_pd_product_link'
  ) THEN
    ALTER TABLE production_demand
      ADD CONSTRAINT chk_pd_product_link
      CHECK ((pd_status = 'PENDING_PRODUCT_LINK') = (pd_cpm_product_sys_id IS NULL));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'chk_pd_link_reason'
  ) THEN
    ALTER TABLE production_demand
      ADD CONSTRAINT chk_pd_link_reason
      CHECK (pd_product_link_reason IS NULL
             OR pd_product_link_reason IN ('AUTO_MATCH_FAILED', 'AMBIGUOUS', 'NO_MASTER_YET'));
  END IF;
END
$$;

COMMENT ON COLUMN production_demand.pd_cpm_product_sys_id IS
  'Soft reference to the finance cost-product-master sys id. NULL only while pd_status = PENDING_PRODUCT_LINK.';
COMMENT ON COLUMN production_demand.pd_product_link_reason IS
  'Why the product is not linked yet: AUTO_MATCH_FAILED (no Orion match), AMBIGUOUS (several matches), NO_MASTER_YET (raised before the master exists). Cleared when the product is linked.';
