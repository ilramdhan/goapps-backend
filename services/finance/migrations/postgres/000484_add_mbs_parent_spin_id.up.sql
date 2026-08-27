-- 000484: jejak duplikasi + recalc pada mst_mb_spin (area D).
--
-- Lima kolom baru, SEMUANYA NULLable, tanpa DEFAULT, tanpa NOT NULL — supaya
-- ALTER TABLE tidak menulis ulang satu baris pun dari ~2679 baris impor Oracle
-- dan updated_at tidak bergerak (U-2).
--
-- Self-FK mbs_parent_spin_id dibuat setelah kolomnya ada. Validasi FK memindai
-- tabel, tetapi karena kolom baru bernilai NULL pada SELURUH baris lama, tidak
-- ada baris yang gagal dan tidak ada baris yang ditulis ulang.
--
-- ⚠ PROTEKSI SIKLUS (R8/G8): FK ini TIDAK mencegah siklus. Bahkan self-loop
-- 1-hop (A -> A) masih mungkin di level DB karena CHECK chk_mbs_parent_not_self
-- yang dirancang design §2.10 SENGAJA TIDAK dibuat di sini — fase P3 ditetapkan
-- "aditif, tanpa constraint CHECK" (plan §5 P3). Sampai CHECK itu lahir di fase
-- berikutnya, larangan A->A DAN siklus multi-hop A->B->A sepenuhnya menjadi
-- tanggung jawab lapisan aplikasi (assertNoParentCycle, walk-up berbatas
-- kedalaman 32, design §4.5). ⛔ Jangan berasumsi DB sudah menjaganya.
--
-- ⛔ NOL UPDATE / NOL backfill: tidak ada baris lama yang ditandai sebagai
-- duplikat maupun sebagai sudah-direcalc.

BEGIN;

ALTER TABLE mst_mb_spin
  ADD COLUMN IF NOT EXISTS mbs_parent_spin_id  UUID,
  ADD COLUMN IF NOT EXISTS mbs_duplicated_at   TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS mbs_duplicated_by   VARCHAR(100),
  ADD COLUMN IF NOT EXISTS mbs_last_recalc_at  TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS mbs_last_recalc_by  VARCHAR(100);

-- Idempoten: ADD CONSTRAINT tidak punya IF NOT EXISTS di PostgreSQL, jadi
-- dijaga dengan DO-block yang memeriksa pg_constraint. Bila constraint sudah
-- ada, blok ini no-op; bila gagal karena sebab lain, ia GAGAL BERISIK dan
-- transaksi dibatalkan (⛔ bukan pola silent-skip ala 000480).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_mbs_parent_spin'
  ) THEN
    ALTER TABLE mst_mb_spin
      ADD CONSTRAINT fk_mbs_parent_spin
        FOREIGN KEY (mbs_parent_spin_id) REFERENCES mst_mb_spin (mbs_id)
        ON DELETE SET NULL;
  END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_mbs_parent_spin
  ON mst_mb_spin (mbs_parent_spin_id) WHERE mbs_parent_spin_id IS NOT NULL;

COMMENT ON COLUMN mst_mb_spin.mbs_parent_spin_id IS
  'Spin sumber tempat baris ini diduplikasi (area D). Self-reference nullable; '
  'siklus dicegah di lapisan aplikasi — DB belum punya CHECK anti self-loop.';
COMMENT ON COLUMN mst_mb_spin.mbs_duplicated_at IS
  'Kapan baris ini dihasilkan lewat aksi duplicate spin. NULL = bukan hasil duplikasi.';
COMMENT ON COLUMN mst_mb_spin.mbs_duplicated_by IS
  'Aktor yang menjalankan duplicate spin. NULL = bukan hasil duplikasi.';
COMMENT ON COLUMN mst_mb_spin.mbs_last_recalc_at IS
  'Kapan spin ini terakhir dihitung ulang. NULL = belum pernah lewat jalur recalc baru.';
COMMENT ON COLUMN mst_mb_spin.mbs_last_recalc_by IS
  'Aktor recalc terakhir. NULL = belum pernah lewat jalur recalc baru.';

COMMIT;
