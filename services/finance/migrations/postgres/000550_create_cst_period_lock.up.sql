-- 000550 — cost period lock (design Part 1 §4.3, §5.5; plan-01 P0-T3).
--
-- A row with cpl_unlocked_at IS NULL = (period, ACTUAL) is frozen for
-- costing: costcalc trigger/verify/approve, mbbatch recompute, mbpush execute
-- and supersedePrevious refuse to change ACTUAL rows of the period (P1).
--
-- Unlock: design §4.3 deletes the row, while plan-01 P0-T3 / plan-02 P1-T4
-- keep it and set unlocked_by/at (P1-T4 guards on `cpl_unlocked_at IS NULL`).
-- The nullable cpl_unlocked_* columns support both; P1 picks one and a
-- re-lock of a soft-unlocked row is an UPDATE of the same PK. Either way the
-- history lives in cost_audit_log (ERP_PERIOD_UNLOCK with the before-snapshot).
-- Scope is ACTUAL only (Q7); widening the CHECK later is additive.
--
-- cpl_erp_batch_id is set at LOCK_BATCH (the ERP freeze). The FK has no
-- cascade: a batch referenced by a lock cannot be deleted.
--
-- New table only; nothing existing changes. Note: the lock does not yet
-- guard anything until the P1 code lands.

BEGIN;

CREATE TABLE IF NOT EXISTS cst_period_lock (
    cpl_period       VARCHAR(6)  NOT NULL,
    cpl_calc_type    VARCHAR(10) NOT NULL,
    cpl_locked_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpl_locked_by    VARCHAR(64) NOT NULL,
    cpl_reason       TEXT        NOT NULL,
    cpl_erp_batch_id BIGINT      NULL
        REFERENCES cst_erp_int_batch (ceib_batch_id),
    cpl_unlocked_at  TIMESTAMPTZ NULL,
    cpl_unlocked_by  VARCHAR(64) NULL,
    cpl_unlock_reason TEXT       NULL,
    CONSTRAINT pk_cst_period_lock PRIMARY KEY (cpl_period, cpl_calc_type),
    CONSTRAINT chk_cpl_period    CHECK (cpl_period ~ '^[0-9]{4}(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_cpl_calc_type CHECK (cpl_calc_type = 'ACTUAL'),
    CONSTRAINT chk_cpl_unlock_pair CHECK ((cpl_unlocked_at IS NULL) = (cpl_unlocked_by IS NULL))
);

COMMENT ON TABLE cst_period_lock IS
    'Cost freeze per (period, ACTUAL) (design §4.3, §5.5). Locked while cpl_unlocked_at IS NULL; history in cost_audit_log ERP_PERIOD_UNLOCK.';

COMMIT;
