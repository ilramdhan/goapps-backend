-- Migration 000368: Seed Global Fill Config (Levels 1-4 + 100-102)
--
-- Dept codes hardcoded from IAM DB (mst_department):
--   FIN  = Finance department
--   PROD = Production department
--
-- Note: mst_department lives in the IAM database (different DB),
-- so cross-DB lookup is not possible. Use actual codes directly.
--
-- Levels 1-4 : Regular route levels (FG + upstream intermediate products).
-- Levels 100-102: Completion approval chain after all route levels are done.
--   100 = Finance dept confirmation (all parameters reviewed)
--   101 = Production dept manager approval
--   102 = Finance dept manager release & lock
--
-- uk_clac_level_active is a partial UNIQUE INDEX on (clac_route_level)
-- WHERE clac_is_active = true.  PostgreSQL requires the WHERE clause to be
-- repeated verbatim in the ON CONFLICT clause for partial indexes.
--
-- clac_sla_approve_hours is NOT NULL (DEFAULT 24).  Levels with no designated
-- approver still carry the column with a sentinel value of 0 to indicate
-- "no approval step" — the application should treat 0 as "no SLA enforced".

INSERT INTO cost_level_assignment_config (
  clac_route_level, clac_filler_type, clac_filler_value,
  clac_approver_type, clac_approver_value,
  clac_reapprove_on_change, clac_sla_fill_hours, clac_sla_approve_hours,
  clac_is_active, clac_created_by, clac_updated_by
) VALUES
  -- Level 1 (FG product): Finance fills, Finance approves
  (1,   'DEPT', 'FIN',  'DEPT', 'FIN',  TRUE,  48, 24, TRUE, 'system_seed', 'system_seed'),
  -- Level 2: Production fills, Production approves
  (2,   'DEPT', 'PROD', 'DEPT', 'PROD', FALSE, 48, 24, TRUE, 'system_seed', 'system_seed'),
  -- Level 3: Production fills, no approver (sla_approve_hours=0 means no SLA)
  (3,   'DEPT', 'PROD', NULL,   NULL,   FALSE, 72,  0, TRUE, 'system_seed', 'system_seed'),
  -- Level 4: Production fills, no approver
  (4,   'DEPT', 'PROD', NULL,   NULL,   FALSE, 72,  0, TRUE, 'system_seed', 'system_seed'),
  -- Level 100: Finance confirmation (after all route levels done)
  (100, 'DEPT', 'FIN',  NULL,   NULL,   FALSE, 48,  0, TRUE, 'system_seed', 'system_seed'),
  -- Level 101: Production Manager approval
  (101, 'DEPT', 'PROD', NULL,   NULL,   FALSE, 24,  0, TRUE, 'system_seed', 'system_seed'),
  -- Level 102: Finance Manager release & lock
  (102, 'DEPT', 'FIN',  NULL,   NULL,   FALSE, 24,  0, TRUE, 'system_seed', 'system_seed')
ON CONFLICT (clac_route_level) WHERE clac_is_active = TRUE DO NOTHING;
