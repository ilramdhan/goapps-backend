-- ============================================================================
-- M-ERP-2: interface user GOAPPS_IF (NEW user)
-- Purpose        : the only account GoApps uses to push std batches and call the package
-- Target schema  : creates user GOAPPS_IF; grants on MGTDAT objects
-- Executed by    : DBA (GoApps never runs this script). Password is supplied at run
--                  time via &&GOAPPS_IF_PWD; there is never a literal password in this file.
-- Prerequisite   : M-ERP-1a, 1b, 1c applied; tablespace (T-1) decided
-- Rollback twin  : M-ERP-2_goapps_if_user_rollback.sql
-- Re-runnable    : CREATE USER ignores ORA-01920; GRANTs are idempotent.
-- Intentionally removed from the lead's section 8 (L-10/L-11, D-S3, I-5): UPDATE on
--   CST_GOAPPS_STD_BATCH (the package is AUTHID DEFINER); SELECT on OT_ADJ_HEAD/ITEM,
--   OM_ITEM, OM_GRADE_CODE_1/2, OT_STD_COST_PRODUCTS_MGT, MGT_ITEM_COST_VAL_LOSS,
--   IM_VS_STATIC_VALUE; SELECT on the V_GOAPPS_* views and ADJ logs; role grants for
--   report readers. Reason: GoApps reads Oracle through the existing read-only user,
--   never through GOAPPS_IF.
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
SET DEFINE ON
DEFINE ts = <TABLESPACE>

BEGIN EXECUTE IMMEDIATE 'CREATE USER GOAPPS_IF IDENTIFIED BY "&&GOAPPS_IF_PWD" DEFAULT TABLESPACE &ts QUOTA 0 ON &ts';
EXCEPTION WHEN OTHERS THEN IF SQLCODE != -1920 THEN RAISE; END IF; END;
/
GRANT CREATE SESSION TO GOAPPS_IF;
GRANT SELECT, INSERT ON MGTDAT.CST_GOAPPS_STD_BATCH TO GOAPPS_IF;
GRANT SELECT, INSERT ON MGTDAT.CST_GOAPPS_STD_COST  TO GOAPPS_IF;
GRANT EXECUTE        ON MGTDAT.PKG_GOAPPS_ADJ       TO GOAPPS_IF;
