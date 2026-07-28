-- Reverse 000037. Sync-sourced rows are dropped first so the NOT NULL
-- constraints can be restored: an MMSMERGE-imported lot may legitimately carry
-- a NULL shade code or standard weight, which the pre-000037 schema forbids.

DELETE FROM lot_master WHERE lm_source = 'MMSMERGE';

DROP INDEX IF EXISTS idx_lot_master_prod_type;
DROP INDEX IF EXISTS idx_lot_master_orion_item;
DROP INDEX IF EXISTS idx_lot_master_source;

ALTER TABLE lot_master DROP CONSTRAINT IF EXISTS chk_lot_master_source;

ALTER TABLE lot_master
    DROP COLUMN IF EXISTS lm_src_pak_status,
    DROP COLUMN IF EXISTS lm_src_status,
    DROP COLUMN IF EXISTS lm_efficiency_pct,
    DROP COLUMN IF EXISTS lm_machine_no,
    DROP COLUMN IF EXISTS lm_orion_item_code,
    DROP COLUMN IF EXISTS lm_src_bob_weight,
    DROP COLUMN IF EXISTS lm_bobbins_per_box,
    DROP COLUMN IF EXISTS lm_tare_bobbin_wt,
    DROP COLUMN IF EXISTS lm_tare_box_wt,
    DROP COLUMN IF EXISTS lm_shade_color,
    DROP COLUMN IF EXISTS lm_description,
    DROP COLUMN IF EXISTS lm_qc_grade,
    DROP COLUMN IF EXISTS lm_cross_section,
    DROP COLUMN IF EXISTS lm_filament,
    DROP COLUMN IF EXISTS lm_denier,
    DROP COLUMN IF EXISTS lm_yarn_type,
    DROP COLUMN IF EXISTS lm_prod_type,
    DROP COLUMN IF EXISTS lm_synced_at,
    DROP COLUMN IF EXISTS lm_source_key,
    DROP COLUMN IF EXISTS lm_source;

-- Backfill before restoring NOT NULL so any remaining row with a NULL left by
-- 000037 does not block the rollback.
UPDATE lot_master SET lm_shade_code = '' WHERE lm_shade_code IS NULL;
UPDATE lot_master SET lm_std_weight_full = 0 WHERE lm_std_weight_full IS NULL;
UPDATE lot_master SET lm_std_weight_unfull = 0 WHERE lm_std_weight_unfull IS NULL;

ALTER TABLE lot_master ALTER COLUMN lm_shade_code        SET NOT NULL;
ALTER TABLE lot_master ALTER COLUMN lm_std_weight_full   SET NOT NULL;
ALTER TABLE lot_master ALTER COLUMN lm_std_weight_unfull SET NOT NULL;
