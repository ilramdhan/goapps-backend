-- Convert percent-stored values to decimal in cst_rm_group_head.
-- Aligns the header convention with cst_rm_group_detail (already decimal)
-- and with the Excel reference (Testing_RM_Cost.xlsx). Calculations across
-- header + detail can then share a single multiplier without inline /100.
--
-- Affected fields:
--   cost_percentage           — V1 marketing duty %  (4 → 0.04)
--   marketing_anti_dumping_pct — V2 marketing anti %  (2 → 0.02)
--
-- NOT affected (already rate / decimal):
--   cost_per_kg, marketing_freight_rate, marketing_transport_rate,
--   marketing_default_value, all valuation_* fields on detail.

UPDATE cst_rm_group_head
   SET cost_percentage = cost_percentage / 100.0
 WHERE cost_percentage IS NOT NULL
   AND cost_percentage > 1; -- guard: skip rows already in decimal form

UPDATE cst_rm_group_head
   SET marketing_anti_dumping_pct = marketing_anti_dumping_pct / 100.0
 WHERE marketing_anti_dumping_pct IS NOT NULL
   AND marketing_anti_dumping_pct > 1; -- guard: skip rows already in decimal form

-- Snapshots in cst_rm_cost copy from the head at calc time, so newly-computed
-- rows will carry the decimal form automatically. Existing snapshot rows get
-- migrated here so the UI shows consistent units before the next recalc.
UPDATE cst_rm_cost
   SET marketing_anti_dumping_pct = marketing_anti_dumping_pct / 100.0
 WHERE marketing_anti_dumping_pct IS NOT NULL
   AND marketing_anti_dumping_pct > 1;

UPDATE cst_rm_cost
   SET marketing_duty_pct = marketing_duty_pct / 100.0
 WHERE marketing_duty_pct IS NOT NULL
   AND marketing_duty_pct > 1;
