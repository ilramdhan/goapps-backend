-- ============================================================================
-- Purpose        : M-ERP-4b: repoint legacy readers to V_GOAPPS_STD_COST_CUR (GUIDED TEMPLATE)
-- Target schema  : MGTDAT
-- M-ERP-4b: repoint legacy readers to V_GOAPPS_STD_COST_CUR (GUIDED TEMPLATE)
-- Executed by    : DBA, at CUTOVER only, after M-ERP-4a. The legacy sources are not in
--                  the repo, so paste the object DDL spooled by 4a into each region.
-- Rollback twin  : M-ERP-4b_repoint_rollback.sql
-- Change rule (every object): replace OT_STD_COST_PRODUCTS_MGT with
--   V_GOAPPS_STD_COST_CUR; column names identical; no other change.
-- NOT repointed (kept as 1-period rollback path): STD_FG_VALUE_INSERT,
--   STD_FG_COST_UPD_PRD, CHP_WAC_UPD, ODBTRG_FG_STD_COST.
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK

-- ---- O_DGET_FG_ITEM_RATE_MGT (cursor FG_COST_PER_KG by item/grade/shade) ----
-- >>> paste the spooled DDL from 4a here and apply the rename <<<
SHOW ERRORS
SELECT NAME, TYPE, LINE, TEXT FROM ALL_ERRORS WHERE OWNER='MGTDAT' AND NAME='O_DGET_FG_ITEM_RATE_MGT';

-- ---- FUNC_CHIP_WAC_MGT (cursor C1; fallback C2 to CST_YARN_CALCULATION_CUR unchanged) ----
-- >>> paste the spooled DDL from 4a here and apply the rename <<<
SHOW ERRORS
SELECT NAME, TYPE, LINE, TEXT FROM ALL_ERRORS WHERE OWNER='MGTDAT' AND NAME='FUNC_CHIP_WAC_MGT';

-- ---- VIEW_MARGIN_REPORT (outer join std) ----
-- >>> paste the spooled DDL from 4a here and apply the rename <<<
SHOW ERRORS
SELECT NAME, TYPE, LINE, TEXT FROM ALL_ERRORS WHERE OWNER='MGTDAT' AND NAME='VIEW_MARGIN_REPORT';

-- ---- VIEW_MARGIN_REPORT_TODAY (outer join std (+ _DEV* variants, T-3)) ----
-- >>> paste the spooled DDL from 4a here and apply the rename <<<
SHOW ERRORS
SELECT NAME, TYPE, LINE, TEXT FROM ALL_ERRORS WHERE OWNER='MGTDAT' AND NAME='VIEW_MARGIN_REPORT_TODAY';

-- ---- PRODUCTS_LIST_COST (OPTIONAL) ----
-- >>> paste the spooled DDL from 4a here and apply the rename <<<
SHOW ERRORS
SELECT NAME, TYPE, LINE, TEXT FROM ALL_ERRORS WHERE OWNER='MGTDAT' AND NAME='PRODUCTS_LIST_COST';
