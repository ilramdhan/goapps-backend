BEGIN;
ALTER TABLE cost_route_head
  ADD COLUMN crh_forked_from_head_id BIGINT
    REFERENCES cost_route_head(crh_head_id) ON DELETE SET NULL;
CREATE INDEX idx_crh_forked_from
  ON cost_route_head (crh_forked_from_head_id)
  WHERE crh_forked_from_head_id IS NOT NULL;
COMMIT;
