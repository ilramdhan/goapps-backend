-- 000617 down — restore the old names from the backup tables, then drop them.
-- Heads/spins edited after 000617 (name no longer equal to its costing) are left alone.
BEGIN;

DO $$
BEGIN
    IF to_regclass('bak_000617_head') IS NOT NULL THEN
        UPDATE mst_mb_head h SET mbh_mgt_name = b.old_name
        FROM bak_000617_head b
        WHERE h.mbh_id = b.mbh_id AND h.mbh_mgt_name IS NOT DISTINCT FROM h.mbh_mb_costing;
    ELSE
        RAISE NOTICE '000617 down: bak_000617_head missing, head names NOT restored';
    END IF;
    IF to_regclass('bak_000617_spin') IS NOT NULL THEN
        UPDATE mst_mb_spin s SET mbs_mgt_name = b.old_name
        FROM bak_000617_spin b, mst_mb_head h
        WHERE s.mbs_id = b.mbs_id AND h.mbh_id = s.mbs_mbh_id
          AND s.mbs_mgt_name IS NOT DISTINCT FROM h.mbh_mb_costing;
    ELSE
        RAISE NOTICE '000617 down: bak_000617_spin missing, spin names NOT restored';
    END IF;
END $$;

DROP TABLE IF EXISTS bak_000617_head;
DROP TABLE IF EXISTS bak_000617_spin;

COMMIT;
