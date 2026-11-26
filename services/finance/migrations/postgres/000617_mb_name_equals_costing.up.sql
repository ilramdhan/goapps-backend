-- 000617 — MB Name (mst_mb_head.mbh_mgt_name) always equals MB Costing Code (mbh_mb_costing).
--
-- User-approved 2026-11-26 (mb-comparison-p2 item 3): costing wins, spins follow the head name
-- (mst_mb_spin.mbs_mgt_name). New writes are enforced in domain/mbhead; this backfills old rows.
-- Only rows that actually change are backed up and updated. updated_at/updated_by are NOT touched
-- (a pure data normalisation, not a user edit). Idempotent: a second run finds nothing to change.
-- Reversible: bak_000617_head / bak_000617_spin hold the old names, read by the down migration.
BEGIN;

CREATE TABLE IF NOT EXISTS bak_000617_head (
    mbh_id       UUID        NOT NULL PRIMARY KEY,
    old_name     VARCHAR(255),
    backed_up_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS bak_000617_spin (
    mbs_id       UUID        NOT NULL PRIMARY KEY,
    old_name     VARCHAR(255),
    backed_up_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
COMMENT ON TABLE bak_000617_head IS 'Backup written by migration 000617; read by its down migration. Safe to drop once 000617 is final.';
COMMENT ON TABLE bak_000617_spin IS 'Backup written by migration 000617; read by its down migration. Safe to drop once 000617 is final.';

INSERT INTO bak_000617_head (mbh_id, old_name)
SELECT mbh_id, mbh_mgt_name FROM mst_mb_head
WHERE deleted_at IS NULL AND mbh_mgt_name IS DISTINCT FROM mbh_mb_costing
ON CONFLICT (mbh_id) DO NOTHING;

INSERT INTO bak_000617_spin (mbs_id, old_name)
SELECT s.mbs_id, s.mbs_mgt_name FROM mst_mb_spin s
JOIN mst_mb_head h ON h.mbh_id = s.mbs_mbh_id
WHERE s.deleted_at IS NULL AND h.deleted_at IS NULL AND s.mbs_mgt_name IS DISTINCT FROM h.mbh_mb_costing
ON CONFLICT (mbs_id) DO NOTHING;

UPDATE mst_mb_head SET mbh_mgt_name = mbh_mb_costing
WHERE deleted_at IS NULL AND mbh_mgt_name IS DISTINCT FROM mbh_mb_costing;

UPDATE mst_mb_spin s SET mbs_mgt_name = h.mbh_mb_costing
FROM mst_mb_head h
WHERE s.mbs_mbh_id = h.mbh_id AND s.deleted_at IS NULL AND h.deleted_at IS NULL
  AND s.mbs_mgt_name IS DISTINCT FROM h.mbh_mb_costing;

COMMIT;
