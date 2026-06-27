DELETE FROM mst_lookup_master_column
WHERE lmc_master_code = 'MACHINE'
  AND lmc_column_name IN ('mp_per_day', 'ohs_per_day', 'spares_per_day', 'kgs_lost_change',
                           'vb1_qty', 'vb2_qty', 'vb3_qty', 'vb4_qty', 'vb5_qty');

ALTER TABLE mst_machine
    DROP COLUMN IF EXISTS mp_per_day,
    DROP COLUMN IF EXISTS ohs_per_day,
    DROP COLUMN IF EXISTS spares_per_day,
    DROP COLUMN IF EXISTS kgs_lost_change,
    DROP COLUMN IF EXISTS vb1_qty,
    DROP COLUMN IF EXISTS vb2_qty,
    DROP COLUMN IF EXISTS vb3_qty,
    DROP COLUMN IF EXISTS vb4_qty,
    DROP COLUMN IF EXISTS vb5_qty;
