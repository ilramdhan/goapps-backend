-- 000520 (oil-cost-rm-group M1) — Oil Group flag on RM group heads.
--
-- Adds cst_rm_group_head.is_oil_group. The flag marks an RM group as an "oil
-- group": only flagged groups can be picked as a product's OIL_NAME (via the
-- RM_GROUP_OIL lookup master / v_rm_group_oil view, 000522) and the calc engine
-- resolves OIL_RATE from that group's per-period cst_rm_cost row.
--
-- The flag is HEAD-LEVEL and GLOBAL, not period-versioned (decision D12): it is
-- deliberately NOT added to cst_rm_group_head_period (000503).
--
-- Seed: the two oil groups confirmed by the user (decision D1):
--   202006101 CONING OIL       -> PTY
--   202006077 SPIN FINISH OIL  -> POY + Superba (TCS/TPS/TTS)
-- Seeding is by group_code only (no literal UUIDs). On a database without these
-- groups (CI / migration-only DB) the UPDATE matches nothing and is a no-op.
--
-- Idempotent: ADD COLUMN IF NOT EXISTS, CREATE INDEX IF NOT EXISTS, and the
-- seed UPDATE only touches rows that are still FALSE.

BEGIN;

ALTER TABLE cst_rm_group_head
    ADD COLUMN IF NOT EXISTS is_oil_group BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN cst_rm_group_head.is_oil_group IS
    'TRUE = oil RM group, selectable as a product OIL_NAME (RM_GROUP_OIL lookup). Head-level/global, not period-versioned.';

UPDATE cst_rm_group_head
SET is_oil_group = TRUE,
    updated_at   = NOW(),
    updated_by   = 'migration_000520'
WHERE group_code IN ('202006101', '202006077')
  AND deleted_at IS NULL
  AND is_oil_group = FALSE;

CREATE INDEX IF NOT EXISTS idx_rm_group_head_is_oil_group
    ON cst_rm_group_head (group_code)
    WHERE is_oil_group AND deleted_at IS NULL;

DO $$
DECLARE
    v_cnt INT;
BEGIN
    SELECT COUNT(*) INTO v_cnt
    FROM cst_rm_group_head
    WHERE is_oil_group AND deleted_at IS NULL;
    RAISE NOTICE '000520: active oil groups flagged = % (expected 2 on a populated DB, 0 on a migration-only DB)', v_cnt;
END $$;

COMMIT;
