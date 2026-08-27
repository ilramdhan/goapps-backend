-- 000482 rollback: buang index pencarian VS Number.
-- Tidak destruktif terhadap data — index adalah struktur turunan.

BEGIN;

DROP INDEX IF EXISTS idx_mst_mb_head_vs_number;

COMMIT;
