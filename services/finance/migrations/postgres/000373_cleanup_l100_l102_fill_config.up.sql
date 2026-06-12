-- Migration 000373: Remove L100/L101/L102 fill config entries.
-- L100-102 triggered the CONFIRM/APPROVE/RELEASE chain from fill config. This is
-- now handled directly via CPR domain Confirm()/Approve()/Release() methods with
-- dedicated permissions. The fill config rows are obsolete.
DELETE FROM cost_level_assignment_config WHERE clac_route_level IN (100, 101, 102);
