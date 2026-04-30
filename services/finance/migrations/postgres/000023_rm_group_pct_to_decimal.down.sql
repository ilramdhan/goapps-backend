-- Reverse migration 000023: decimal → percent.
-- Multiplies fields by 100 only when value is in decimal form (≤ 1).

UPDATE cst_rm_group_head
   SET cost_percentage = cost_percentage * 100.0
 WHERE cost_percentage IS NOT NULL
   AND cost_percentage <= 1;

UPDATE cst_rm_group_head
   SET marketing_anti_dumping_pct = marketing_anti_dumping_pct * 100.0
 WHERE marketing_anti_dumping_pct IS NOT NULL
   AND marketing_anti_dumping_pct <= 1;

UPDATE cst_rm_cost
   SET marketing_anti_dumping_pct = marketing_anti_dumping_pct * 100.0
 WHERE marketing_anti_dumping_pct IS NOT NULL
   AND marketing_anti_dumping_pct <= 1;

UPDATE cst_rm_cost
   SET marketing_duty_pct = marketing_duty_pct * 100.0
 WHERE marketing_duty_pct IS NOT NULL
   AND marketing_duty_pct <= 1;
