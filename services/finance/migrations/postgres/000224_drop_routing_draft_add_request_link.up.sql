BEGIN;

ALTER TABLE cost_product_request
  ADD COLUMN cpr_linked_route_head_id BIGINT
    REFERENCES cost_route_head(crh_head_id) ON DELETE SET NULL;

CREATE INDEX idx_cpr_linked_route_head
  ON cost_product_request (cpr_linked_route_head_id)
  WHERE cpr_linked_route_head_id IS NOT NULL;

-- Backfill from existing draft→head chain.
UPDATE cost_product_request cpr
SET    cpr_linked_route_head_id = crd.crd_linked_route_head_id
FROM   cost_routing_draft crd
WHERE  crd.crd_request_id = cpr.cpr_request_id
  AND  crd.crd_linked_route_head_id IS NOT NULL
  AND  crd.crd_status = 'PROMOTED';

DROP TABLE IF EXISTS cost_routing_draft_component CASCADE;
DROP TABLE IF EXISTS cost_routing_draft           CASCADE;

COMMIT;
