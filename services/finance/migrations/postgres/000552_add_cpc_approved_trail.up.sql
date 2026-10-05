-- 000552 — approval trail on cst_product_cost (design Part 1 §4.5, M-4, N-3;
-- plan-01 P0-T4).
--
-- Today approve overwrites cpc_verified_at/by, so an approved row has no
-- separate approval timestamp. This adds cpc_approved_at/by (written by the
-- approve path from P0-T8 on; cpc_verified_* writes stay byte-identical) and
-- backfills existing APPROVED rows from cpc_verified_*.
--
-- ACCEPTED LOSS (documented): for rows approved before this migration the
-- true historical verify timestamp is already gone (it was overwritten on
-- approve), so the backfilled approved_* equals verified_* for those rows.
--
-- cpc_verified_* is NOT modified. Only APPROVED rows with NULL approved_at are
-- touched; the DO block asserts that none remain. The new columns are not
-- mapped to any API response by this migration.
--
-- cpc_approved_by is VARCHAR(100) to match cpc_verified_by (000228), so the
-- backfill can never truncate or fail (design §4.5 says 64; widened on
-- purpose).

BEGIN;

ALTER TABLE cst_product_cost
    ADD COLUMN IF NOT EXISTS cpc_approved_at TIMESTAMPTZ  NULL,
    ADD COLUMN IF NOT EXISTS cpc_approved_by VARCHAR(100) NULL;

COMMENT ON COLUMN cst_product_cost.cpc_approved_at IS 'Approval timestamp (000552). Backfilled from cpc_verified_at for rows approved before 000552.';
COMMENT ON COLUMN cst_product_cost.cpc_approved_by IS 'Approver (000552). Backfilled from cpc_verified_by for rows approved before 000552.';

UPDATE cst_product_cost
   SET cpc_approved_at = cpc_verified_at,
       cpc_approved_by = cpc_verified_by
 WHERE cpc_status = 'APPROVED'
   AND cpc_approved_at IS NULL;

DO $$
DECLARE
    v_left INT;
BEGIN
    -- An APPROVED row whose cpc_verified_at was NULL would stay NULL here.
    SELECT COUNT(*) INTO v_left
      FROM cst_product_cost
     WHERE cpc_status = 'APPROVED' AND cpc_approved_at IS NULL;
    IF v_left > 0 THEN
        RAISE EXCEPTION '000552: % APPROVED cst_product_cost row(s) still have NULL cpc_approved_at (their cpc_verified_at is NULL)', v_left;
    END IF;
END $$;

COMMIT;
