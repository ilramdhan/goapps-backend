-- ============================================================================
-- M-ERP-1c: package MGTDAT.PKG_GOAPPS_ADJ (the only write path into OT_ADJ_*)
-- Purpose        : create the lead's package spec + body (NEW object, copied unchanged)
-- Target schema  : MGTDAT (Oracle 11.2.0.4)
-- Executed by    : DBA (GoApps never runs this script)
-- Prerequisite   : M-ERP-1a (tables, ADJ logs) and M-ERP-1b (views) applied
-- Rollback twin  : M-ERP-1c_pkg_goapps_adj_rollback.sql
-- Re-runnable    : CREATE OR REPLACE. COMMIT is done by GoApps after a successful call.
-- Source         : ddl_proposal_mgtdat_interface.sql l.217-408 (user decision U-1:
--                  period-wide, P_BATCH_ID only). The section below the separator is
--                  byte-for-byte the lead's code; do NOT edit it.
--
-- Non-blocking suggestions for the lead (none blocks deploy):
--   (a) use NVL(ADJH_APPR_STATUS,0) <> 3 instead of ADJH_APPR_STATUS != 3
--       (a NULL status is silently excluded today).
--   (b) VALUATE: use LEFT JOIN instead of INNER JOIN to OM_ITEM (C-10: if the item is
--       missing the correlated subquery returns no row and NULLs every column).
--   (c) no UPDATE grant on CST_GOAPPS_STD_BATCH is needed for GOAPPS_IF (AUTHID DEFINER).
--
-- GoApps-side compensating gates: G6 whole-set eligibility, G7 PG snapshot of all
--   affected rows, G7a V-07 pre-validation, G8 NOWAIT/timeout run outside the WMS peak
--   (accepted lock risk), G9 idempotent re-run, call log.
--
-- ORA codes raised by this package (and the guard triggers of M-ERP-1a):
--   -20901  period already has posted ADJ heads; integration refused
--   -20902  rate > 20 for prefixes guarded by ODBTRG_COST_VAL_MGT
--   -20903  batch incomplete/inconsistent (row count or control sums differ)
--   -20904  LOCK_BATCH: period has no posted ADJ yet
--   -20905  batch status wrong (not PUSHED in VALUATE; not VALUATED/APPROVED in LOCK)
--   -20910  guard trigger: STD_BATCH/STD_COST changed by a user other than GOAPPS_IF
--   -20911  guard trigger: STD_COST is insert-only; correction = new batch
--   -20912  guard trigger: STD_COST row inserted into a batch whose status is not PUSHED
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
-- ---------------------------------------------------------------------------
-- BEGIN lead's code (unchanged)
-- ---------------------------------------------------------------------------
CREATE OR REPLACE PACKAGE MGTDAT.PKG_GOAPPS_ADJ AUTHID DEFINER AS
  -- goapps: INSERT batch (status PUSHED) + baris std, lalu panggil ini
  PROCEDURE VALUATE_ADJ (P_BATCH_ID IN NUMBER, P_SUMMARY OUT VARCHAR2);
  -- opsional (D-8): approve head yang semua itemnya dinilai batch ini, rate > 0
  PROCEDURE APPROVE_ADJ (P_BATCH_ID IN NUMBER, P_APPR_UID IN VARCHAR2, P_SUMMARY OUT VARCHAR2);
  -- rollback sebelum posting: kembalikan nilai ADJ dari log batch
  PROCEDURE RESTORE_ADJ (P_BATCH_ID IN NUMBER, P_SUMMARY OUT VARCHAR2);
  -- setelah posting: tandai batch LOCKED (FR-8)
  PROCEDURE LOCK_BATCH  (P_BATCH_ID IN NUMBER, P_SUMMARY OUT VARCHAR2);
END PKG_GOAPPS_ADJ;
/

CREATE OR REPLACE PACKAGE BODY MGTDAT.PKG_GOAPPS_ADJ AS

  -- v0.3.1: yarn (INVADJ) dan MB (MBINVADJ/MBINVADJRP) sama-sama dinilai dari batch goapps (OQ-5, D-7).

  FUNCTION period_start(p VARCHAR2) RETURN DATE IS BEGIN RETURN TO_DATE(p||'01','YYYYMMDD'); END;
  FUNCTION period_end  (p VARCHAR2) RETURN DATE IS BEGIN RETURN LAST_DAY(TO_DATE(p||'01','YYYYMMDD')); END;

  FUNCTION batch_period(P_BATCH_ID NUMBER) RETURN VARCHAR2 IS
    v VARCHAR2(6);
  BEGIN
    SELECT GSB_PERIOD INTO v FROM CST_GOAPPS_STD_BATCH WHERE GSB_BATCH_ID = P_BATCH_ID;
    RETURN v;
  END;

  FUNCTION posted_heads(p VARCHAR2) RETURN NUMBER IS
    n NUMBER;
  BEGIN
    SELECT COUNT(*) INTO n FROM OT_ADJ_HEAD
     WHERE ADJH_TXN_CODE IN ('INVADJ','MBINVADJ','MBINVADJRP')
       AND TRUNC(ADJH_DT) BETWEEN period_start(p) AND period_end(p)
       AND ADJH_POST_STATUS IS NOT NULL;
    RETURN n;
  END;

  PROCEDURE assert_not_posted(p VARCHAR2) IS
    n NUMBER := posted_heads(p);
  BEGIN
    IF n > 0 THEN
      RAISE_APPLICATION_ERROR(-20901, 'Periode '||p||' sudah ada ADJ posted ('||n||' head). Integrasi ditolak.');
    END IF;
  END;

  -- total kontrol: duplikat Oracle harus sama dengan yang dideklarasikan goapps (PRD §11.4)
  PROCEDURE assert_batch_complete(P_BATCH_ID NUMBER) IS
    b CST_GOAPPS_STD_BATCH%ROWTYPE;
    n NUMBER; s_std NUMBER; s_conv NUMBER; s_pvl NUMBER;
  BEGIN
    SELECT * INTO b FROM CST_GOAPPS_STD_BATCH WHERE GSB_BATCH_ID = P_BATCH_ID;
    IF b.GSB_STATUS <> 'PUSHED' THEN
      RAISE_APPLICATION_ERROR(-20905, 'Batch '||P_BATCH_ID||' berstatus '||b.GSB_STATUS||', bukan PUSHED.');
    END IF;
    SELECT COUNT(*), NVL(SUM(GSC_STD_COST),0), NVL(SUM(NVL(GSC_CONV_COST,0)),0), NVL(SUM(NVL(GSC_PROD_VAL_LOSS,0)),0)
      INTO n, s_std, s_conv, s_pvl
      FROM CST_GOAPPS_STD_COST WHERE GSC_BATCH_ID = P_BATCH_ID;
    IF n <> b.GSB_ROW_COUNT OR s_std <> b.GSB_SUM_STD OR s_conv <> b.GSB_SUM_CONV OR s_pvl <> b.GSB_SUM_PVL THEN
      RAISE_APPLICATION_ERROR(-20903, 'Batch '||P_BATCH_ID||' tidak lengkap/tidak konsisten: rows '||n||'/'||b.GSB_ROW_COUNT
                                      ||', sum_std '||s_std||'/'||b.GSB_SUM_STD);
    END IF;
  END;

  PROCEDURE VALUATE_ADJ(P_BATCH_ID IN NUMBER, P_SUMMARY OUT VARCHAR2) IS
    p VARCHAR2(6) := batch_period(P_BATCH_ID);
    n_upd NUMBER; n_nocov NUMBER; n_bad NUMBER;
  BEGIN
    assert_not_posted(p);
    assert_batch_complete(P_BATCH_ID);

    -- tolak rate > 20 untuk prefix yang dijaga trigger ODBTRG_COST_VAL_MGT (goapps sudah validasi V-07)
    SELECT COUNT(*) INTO n_bad FROM CST_GOAPPS_STD_COST
     WHERE GSC_BATCH_ID = P_BATCH_ID AND GSC_STD_COST > 20
       AND SUBSTR(GSC_ITEM_CODE,1,3) IN ('POY','PTY','ACY','ITY','MMK','TTY','HOY');
    IF n_bad > 0 THEN
      RAISE_APPLICATION_ERROR(-20902, n_bad||' rate > 20 di batch '||P_BATCH_ID);
    END IF;

    -- log nilai lama untuk baris yang akan diubah
    INSERT INTO CST_GOAPPS_ADJ_LOG (GAL_ID, GAL_BATCH_ID, GAL_ADJI_SYS_ID, OLD_RATE, OLD_VAL,
           OLD_FLEX_01, OLD_FLEX_02, OLD_FLEX_03, OLD_FLEX_04, OLD_FLEX_05, OLD_FLEX_06, OLD_FLEX_07,
           OLD_FLEX_08, OLD_FLEX_09, OLD_FLEX_10, OLD_FLEX_11, OLD_FLEX_12, OLD_FLEX_13, OLD_FLEX_14)
    SELECT SEQ_GOAPPS_ADJ_LOG.NEXTVAL, P_BATCH_ID, i.ADJI_SYS_ID, i.ADJI_RATE, i.ADJI_VAL,
           i.ADJI_FLEX_01, i.ADJI_FLEX_02, i.ADJI_FLEX_03, i.ADJI_FLEX_04, i.ADJI_FLEX_05, i.ADJI_FLEX_06, i.ADJI_FLEX_07,
           i.ADJI_FLEX_08, i.ADJI_FLEX_09, i.ADJI_FLEX_10, i.ADJI_FLEX_11, i.ADJI_FLEX_12, i.ADJI_FLEX_13, i.ADJI_FLEX_14
      FROM OT_ADJ_HEAD h JOIN OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
     WHERE h.ADJH_TXN_CODE IN ('INVADJ','MBINVADJ','MBINVADJRP')
       AND TRUNC(h.ADJH_DT) BETWEEN period_start(p) AND period_end(p)
       AND h.ADJH_APPR_STATUS != 3 AND h.ADJH_POST_STATUS IS NULL
       AND EXISTS (SELECT 1 FROM CST_GOAPPS_STD_COST s
                    WHERE s.GSC_BATCH_ID = P_BATCH_ID AND s.GSC_ITEM_CODE = i.ADJI_ITEM_CODE
                      AND s.GSC_GRADE_CODE = i.ADJI_GRADE_CODE_1 AND s.GSC_SHADE_CODE = i.ADJI_GRADE_CODE_2);

    -- hanya baris yang punya std di batch; baris tanpa std TIDAK di-reset (dilaporkan NOT_COVERED)
    -- format flex ditulis inline: fungsi privat package tidak bisa dipanggil dari SQL (PLS-00231)
    UPDATE OT_ADJ_ITEM i
       SET (ADJI_ITEM_DESC, ADJI_RATE, ADJI_VAL,
            ADJI_FLEX_01, ADJI_FLEX_02, ADJI_FLEX_03, ADJI_FLEX_04, ADJI_FLEX_05, ADJI_FLEX_06,
            ADJI_FLEX_07, ADJI_FLEX_08, ADJI_FLEX_09, ADJI_FLEX_10, ADJI_FLEX_11, ADJI_FLEX_12,
            ADJI_FLEX_13, ADJI_FLEX_14) =
           (SELECT it.ITEM_NAME,
                   s.GSC_STD_COST,
                   ROUND(i.ADJI_QTY_BU/1000 * s.GSC_STD_COST, 7),
                   TO_CHAR(ROUND(NVL(s.GSC_CONV_COST,0),5),'FM990D00000'),
                   TO_CHAR(ROUND(NVL(s.GSC_CHP_CON_KG,0),5),'FM990D00000'),
                   TO_CHAR(ROUND(NVL(s.GSC_CHP_COST,0),5),'FM990D00000'),
                   s.GSC_CHP_ITEM_CODE, s.GSC_FG_TYPE, s.GSC_BASIS,
                   TO_CHAR(ROUND(NVL(s.GSC_SELLING_PRICE,0),5),'FM990D00000'),
                   TO_CHAR(ROUND(NVL(s.GSC_AX_COST,0),5),'FM990D00000'),
                   TO_CHAR(ROUND(NVL(s.GSC_AX_CONV_COST,0),5),'FM990D00000'),
                   TO_CHAR(ROUND(NVL(s.GSC_VALUE_LOSS,0),5),'FM990D00000'),
                   TO_CHAR(ROUND(NVL(s.GSC_PROD_VAL_LOSS,0),5),'FM990D00000'),
                   s.GSC_MS_BATCH_ITEM,
                   TO_CHAR(P_BATCH_ID), s.GSC_SOURCE                        -- D-6 (OQ-E disetujui)
              FROM CST_GOAPPS_STD_COST s
              JOIN OM_ITEM it ON it.ITEM_CODE = s.GSC_ITEM_CODE
             WHERE s.GSC_BATCH_ID = P_BATCH_ID AND s.GSC_ITEM_CODE = i.ADJI_ITEM_CODE
               AND s.GSC_GRADE_CODE = i.ADJI_GRADE_CODE_1 AND s.GSC_SHADE_CODE = i.ADJI_GRADE_CODE_2)
     WHERE i.ADJI_SYS_ID IN (SELECT l.GAL_ADJI_SYS_ID FROM CST_GOAPPS_ADJ_LOG l WHERE l.GAL_BATCH_ID = P_BATCH_ID);
    n_upd := SQL%ROWCOUNT;

    SELECT COUNT(*) INTO n_nocov
      FROM OT_ADJ_HEAD h JOIN OT_ADJ_ITEM i ON i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
     WHERE h.ADJH_TXN_CODE IN ('INVADJ','MBINVADJ','MBINVADJRP')
       AND TRUNC(h.ADJH_DT) BETWEEN period_start(p) AND period_end(p)
       AND h.ADJH_APPR_STATUS != 3 AND h.ADJH_POST_STATUS IS NULL
       AND NVL(i.ADJI_FLEX_13,'-') <> TO_CHAR(P_BATCH_ID);

    UPDATE CST_GOAPPS_STD_BATCH SET GSB_STATUS = 'SUPERSEDED'
     WHERE GSB_PERIOD = p AND GSB_BATCH_ID <> P_BATCH_ID AND GSB_STATUS NOT IN ('SUPERSEDED','FAILED');
    P_SUMMARY := '{"adj_items_updated":'||n_upd||',"adj_items_not_covered":'||n_nocov||'}';
    UPDATE CST_GOAPPS_STD_BATCH
       SET GSB_STATUS = 'VALUATED', GSB_VALUATED_DT = SYSDATE, GSB_SUMMARY = P_SUMMARY
     WHERE GSB_BATCH_ID = P_BATCH_ID;
  END VALUATE_ADJ;

  PROCEDURE APPROVE_ADJ(P_BATCH_ID IN NUMBER, P_APPR_UID IN VARCHAR2, P_SUMMARY OUT VARCHAR2) IS
    p VARCHAR2(6) := batch_period(P_BATCH_ID);
    n NUMBER;
  BEGIN
    assert_not_posted(p);
    UPDATE OT_ADJ_HEAD h
       SET ADJH_APPR_STATUS = 3, ADJH_APPR_UID = P_APPR_UID, ADJH_APPR_DT = SYSDATE
     WHERE h.ADJH_TXN_CODE IN ('INVADJ','MBINVADJ','MBINVADJRP')
       AND TRUNC(h.ADJH_DT) BETWEEN period_start(p) AND period_end(p)
       AND h.ADJH_APPR_STATUS != 3
       AND NOT EXISTS (SELECT 1 FROM OT_ADJ_ITEM i
                        WHERE i.ADJI_ADJH_SYS_ID = h.ADJH_SYS_ID
                          AND (NVL(i.ADJI_RATE,0) <= 0 OR NVL(i.ADJI_FLEX_13,'-') <> TO_CHAR(P_BATCH_ID)));
    n := SQL%ROWCOUNT;
    UPDATE CST_GOAPPS_STD_BATCH SET GSB_STATUS = 'APPROVED' WHERE GSB_BATCH_ID = P_BATCH_ID;
    P_SUMMARY := '{"adj_heads_approved":'||n||'}';
  END APPROVE_ADJ;

  PROCEDURE RESTORE_ADJ(P_BATCH_ID IN NUMBER, P_SUMMARY OUT VARCHAR2) IS
    p VARCHAR2(6) := batch_period(P_BATCH_ID);
    n NUMBER;
  BEGIN
    assert_not_posted(p);
    UPDATE OT_ADJ_ITEM i
       SET (ADJI_RATE, ADJI_VAL, ADJI_FLEX_01, ADJI_FLEX_02, ADJI_FLEX_03, ADJI_FLEX_04, ADJI_FLEX_05,
            ADJI_FLEX_06, ADJI_FLEX_07, ADJI_FLEX_08, ADJI_FLEX_09, ADJI_FLEX_10, ADJI_FLEX_11, ADJI_FLEX_12,
            ADJI_FLEX_13, ADJI_FLEX_14) =
           (SELECT l.OLD_RATE, l.OLD_VAL, l.OLD_FLEX_01, l.OLD_FLEX_02, l.OLD_FLEX_03, l.OLD_FLEX_04, l.OLD_FLEX_05,
                   l.OLD_FLEX_06, l.OLD_FLEX_07, l.OLD_FLEX_08, l.OLD_FLEX_09, l.OLD_FLEX_10, l.OLD_FLEX_11, l.OLD_FLEX_12,
                   l.OLD_FLEX_13, l.OLD_FLEX_14
              FROM CST_GOAPPS_ADJ_LOG l
             WHERE l.GAL_BATCH_ID = P_BATCH_ID AND l.GAL_ADJI_SYS_ID = i.ADJI_SYS_ID)
     WHERE i.ADJI_SYS_ID IN (SELECT GAL_ADJI_SYS_ID FROM CST_GOAPPS_ADJ_LOG WHERE GAL_BATCH_ID = P_BATCH_ID)
       AND i.ADJI_FLEX_13 = TO_CHAR(P_BATCH_ID);        -- jangan timpa hasil batch yang lebih baru
    n := SQL%ROWCOUNT;
    -- FAILED -> batch keluar dari V_GOAPPS_STD_COST_CUR; batch sebelumnya TIDAK otomatis aktif lagi
    -- (tetap SUPERSEDED). Bila perlu, push ulang batch dari goapps.
    UPDATE CST_GOAPPS_STD_BATCH SET GSB_STATUS = 'FAILED', GSB_ERROR = 'RESTORED' WHERE GSB_BATCH_ID = P_BATCH_ID;
    P_SUMMARY := '{"adj_items_restored":'||n||'}';
  END RESTORE_ADJ;

  PROCEDURE LOCK_BATCH(P_BATCH_ID IN NUMBER, P_SUMMARY OUT VARCHAR2) IS
    p VARCHAR2(6) := batch_period(P_BATCH_ID);
    n NUMBER := posted_heads(p);
  BEGIN
    IF n = 0 THEN
      RAISE_APPLICATION_ERROR(-20904, 'Periode '||p||' belum ada ADJ posted; lock ditolak.');
    END IF;
    UPDATE CST_GOAPPS_STD_BATCH SET GSB_STATUS = 'LOCKED', GSB_LOCKED_DT = SYSDATE
     WHERE GSB_BATCH_ID = P_BATCH_ID AND GSB_STATUS IN ('VALUATED','APPROVED');
    IF SQL%ROWCOUNT = 0 THEN
      RAISE_APPLICATION_ERROR(-20905, 'Batch '||P_BATCH_ID||' bukan VALUATED/APPROVED.');
    END IF;
    P_SUMMARY := '{"locked":1,"posted_heads":'||n||'}';
  END LOCK_BATCH;

END PKG_GOAPPS_ADJ;
