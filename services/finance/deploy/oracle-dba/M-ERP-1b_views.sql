-- ============================================================================
-- M-ERP-1b: GoApps views (read-only SELECT of legacy OT_ADJ_*, OT_STD_COST_PRODUCTS_MGT)
-- Purpose        : V_GOAPPS_ADJ_DEMAND, V_GOAPPS_STD_COST_LATEST, V_GOAPPS_STD_COST_CUR
-- Target schema  : MGTDAT
-- Executed by    : DBA (GoApps never runs this script)
-- Prerequisite   : M-ERP-1a_interface_tables.sql
-- Rollback twin  : M-ERP-1b_views_rollback.sql
-- Re-runnable    : CREATE OR REPLACE (new views only). No DML, no COMMIT.
-- T-3 (flagged for checklist): the VIEW_MARGIN_REPORT_DEV* margin variants are NOT
--   covered here; they are repointed in M-ERP-4b after the DBA answers T-3.
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK

-- 5a. ADJ demand (ETL + recon). GoApps ALWAYS filters PERIOD = :P (L-1).
--     If slow, use a TRUNC(ADJH_DT) range query instead (DATA_MAPPING_SPEC s2.1).
CREATE OR REPLACE VIEW MGTDAT.V_GOAPPS_ADJ_DEMAND AS
SELECT TO_CHAR(h.ADJH_DT,'YYYYMM')       PERIOD,
       h.ADJH_TXN_CODE                   TXN_CODE,
       i.ADJI_ITEM_CODE                  ITEM_CODE,
       i.ADJI_GRADE_CODE_1               GRADE_CODE,
       i.ADJI_GRADE_CODE_2               SHADE_CODE,
       COUNT(*)                          ITEM_COUNT,
       COUNT(DISTINCT h.ADJH_SYS_ID)     HEAD_COUNT,
       SUM(i.ADJI_QTY_BU)/1000           QTY_KG,
       COUNT(DISTINCT i.ADJI_RATE)       RATE_VARIANTS,
       MIN(i.ADJI_RATE)                  MIN_RATE,
       MAX(i.ADJI_RATE)                  MAX_RATE,
       SUM(i.ADJI_VAL)                   ADJ_VAL,
       SUM(CASE WHEN h.ADJH_APPR_STATUS = 3 THEN 1 ELSE 0 END)          APPROVED_ITEMS,
       SUM(CASE WHEN h.ADJH_POST_STATUS IS NOT NULL THEN 1 ELSE 0 END)  POSTED_ITEMS,
       MAX(i.ADJI_FLEX_13)               GOAPPS_BATCH,
       MAX(i.ADJI_FLEX_14)               GOAPPS_SOURCE
FROM   MGTDAT.OT_ADJ_HEAD h
JOIN   MGTDAT.OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
WHERE  h.ADJH_TXN_CODE IN ('INVADJ','MBINVADJ','MBINVADJRP')
GROUP BY TO_CHAR(h.ADJH_DT,'YYYYMM'), h.ADJH_TXN_CODE,
         i.ADJI_ITEM_CODE, i.ADJI_GRADE_CODE_1, i.ADJI_GRADE_CODE_2;

-- 5b. Latest GoApps std per (item, grade, shade): newest period/batch among valid batches
--     (carry-forward of combinations not produced this month). GoApps data only.
CREATE OR REPLACE VIEW MGTDAT.V_GOAPPS_STD_COST_LATEST AS
SELECT *
  FROM (SELECT s.*, b.GSB_STATUS,
               ROW_NUMBER() OVER (PARTITION BY s.GSC_ITEM_CODE, s.GSC_GRADE_CODE, s.GSC_SHADE_CODE
                                  ORDER BY s.GSC_PERIOD DESC, s.GSC_BATCH_ID DESC) rn
          FROM MGTDAT.CST_GOAPPS_STD_COST s
          JOIN MGTDAT.CST_GOAPPS_STD_BATCH b ON b.GSB_BATCH_ID = s.GSC_BATCH_ID
         WHERE b.GSB_STATUS IN ('VALUATED','APPROVED','LOCKED'))
 WHERE rn = 1;

-- 5c. Replacement for reads of OT_STD_COST_PRODUCTS_MGT (legacy column names, so a repoint
--     only swaps the object name). Legacy rows never pushed by GoApps appear as
--     SOURCE='LEGACY_FROZEN' (L-2: keep) ONLY for margin reports / initial WMS rate.
--     ADJ valuation does not use this view.
CREATE OR REPLACE VIEW MGTDAT.V_GOAPPS_STD_COST_CUR AS
SELECT g.GSC_ITEM_CODE                            FG_ITEM_CODE,
       g.GSC_GRADE_CODE                           FG_ITEM_GRADE,
       g.GSC_SHADE_CODE                           FG_ITEM_SHADE,
       g.GSC_STD_COST                             FG_COST_PER_KG,
       g.GSC_CONV_COST                            FG_CONVER_COST,
       NVL(g.GSC_CONV_COST1, g.GSC_CONV_COST)     FG_CONVER_COST1,   -- NVL: without it margin "FG Cost" = NULL
       NVL(g.GSC_CONV_COST2, g.GSC_CONV_COST)     FG_CONVER_COST2,
       NVL(g.GSC_CONV_COST4, g.GSC_CONV_COST)     FG_CONVER_COST4,
       NVL(g.GSC_CONV_COST5, g.GSC_CONV_COST)     FG_CONVER_COST5,
       g.GSC_CHP_ITEM_CODE                        FG_CHP_ITEM_CODE,
       g.GSC_CHP_CON_KG                           FG_CHP_CON_KG,
       g.GSC_CHP_COST                             FG_CHP_COST,
       g.GSC_FG_TYPE                              FG_TYPE,
       g.GSC_BASIS                                FG_BASIS,
       g.GSC_PROD_VAL_LOSS                        FG_PROD_VALUE_LOSS,
       g.GSC_MS_BATCH_ITEM                        FG_MS_BATCH_ITEM,
       g.GSC_ITEM_TYPE                            FG_ITEM_TYPE,
       g.GSC_PRD_PER_DAY                          FG_PRD_PER_DAY,
       g.GSC_SOURCE                               SOURCE,
       g.GSC_PERIOD                               PERIOD,
       g.GSC_BATCH_ID                             BATCH_ID
  FROM MGTDAT.V_GOAPPS_STD_COST_LATEST g
UNION ALL
SELECT o.FG_ITEM_CODE, o.FG_ITEM_GRADE, o.FG_ITEM_SHADE,
       o.FG_COST_PER_KG, o.FG_CONVER_COST,
       o.FG_CONVER_COST1, o.FG_CONVER_COST2, o.FG_CONVER_COST4, o.FG_CONVER_COST5,
       o.FG_CHP_ITEM_CODE, o.FG_CHP_CON_KG, o.FG_CHP_COST,
       o.FG_TYPE, o.FG_BASIS, o.FG_PROD_VALUE_LOSS, o.FG_MS_BATCH_ITEM,
       o.FG_ITEM_TYPE, o.FG_PRD_PER_DAY,
       'LEGACY_FROZEN', NULL, NULL
  FROM MGTDAT.OT_STD_COST_PRODUCTS_MGT o
 WHERE NOT EXISTS (SELECT 1 FROM MGTDAT.CST_GOAPPS_STD_COST s
                    JOIN MGTDAT.CST_GOAPPS_STD_BATCH b ON b.GSB_BATCH_ID = s.GSC_BATCH_ID
                   WHERE b.GSB_STATUS IN ('VALUATED','APPROVED','LOCKED')
                     AND s.GSC_ITEM_CODE = o.FG_ITEM_CODE AND s.GSC_GRADE_CODE = o.FG_ITEM_GRADE
                     AND s.GSC_SHADE_CODE = o.FG_ITEM_SHADE);
-- Note: if legacy column types differ (e.g. FG_PRD_PER_DAY VARCHAR2), adjust GSC_* or CAST here.
-- Note: only the FG_* columns used by the M-ERP-4b reader objects are exposed.
