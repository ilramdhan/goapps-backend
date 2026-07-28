-- 000033: let one work order cover several plan items (WO merge).
--
-- Several contracts of the same product frequently differ only in the
-- finished-goods shade. At the upstream (tty/pty/poy) levels the route product
-- is natural, so those plan items are physically the same work and should be
-- produced as ONE work order with the quantities summed.
--
-- wo_plan_item_link (created in 000013) already carries the full set with a
-- per-item wpl_qty_contribution and UNIQUE (wpl_wo_id, wpl_plan_item_id).
-- Only the write path was missing.
--
-- wo_plan_item_id is deliberately RETAINED as the anchor plan item so existing
-- joins keep working unchanged (notably ListForGantt, which joins the WO to a
-- single plan item to place the bar). It becomes nullable so a future
-- link-only work order is representable, but every WO created today still
-- writes it.

ALTER TABLE work_order ALTER COLUMN wo_plan_item_id DROP NOT NULL;

-- A plan item may be covered by AT MOST ONE work order. The pre-existing
-- uq_wpl_wo_plan_item only prevents the same item joining the same WO twice;
-- this is what actually prevents double-planning the same demand across two
-- work orders. Enforced in the database so a race between two planners cannot
-- slip past the application check.
CREATE UNIQUE INDEX IF NOT EXISTS uq_wpl_plan_item
  ON wo_plan_item_link (wpl_plan_item_id);

COMMENT ON INDEX uq_wpl_plan_item IS
  'A plan item can be linked to at most one work order — no double-linking.';

COMMENT ON COLUMN work_order.wo_plan_item_id IS
  'Anchor plan item: the one the planner started from. Retained (and still populated) so single-plan-item joins such as ListForGantt keep working. The authoritative, complete set of plan items covered by this WO lives in wo_plan_item_link, which also holds each item''s qty contribution. Nullable only to allow a link-only WO; do not read it as "the sole plan item".';
