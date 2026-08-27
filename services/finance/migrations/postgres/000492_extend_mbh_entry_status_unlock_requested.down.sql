-- Down 000492: kembalikan CHECK `mbh_entry_status` ke TUJUH nilai (tanpa
--              'UNLOCK_REQUESTED'), yaitu keadaan setelah 000488.
--
-- ⚠ ROLLBACK INI AKAN GAGAL — SENGAJA — BILA SUDAH ADA BARIS BERSTATUS
-- 'UNLOCK_REQUESTED'. PostgreSQL memvalidasi CHECK baru terhadap SELURUH baris
-- tabel; satu baris 'UNLOCK_REQUESTED' saja akan menolak ADD CONSTRAINT dan
-- seluruh transaksi ini dibatalkan.
--
-- 🔴 ITU PERILAKU YANG BENAR, ⛔ BUKAN BUG. Menyempitkan enum sementara datanya
-- masih memakai nilai yang dibuang berarti nasib data itu harus diputuskan LEBIH
-- DULU OLEH USER: head yang sedang menunggu keputusan unlock mau dikembalikan ke
-- APPROVED? ke VALIDATED? ke DRAFT? Migrasi tidak boleh memilih sendiri.
-- ⛔ JANGAN menambahkan `UPDATE` di berkas ini untuk "membersihkan" baris itu —
-- itu berarti migrasi diam-diam mengubah status kerja orang (prinsip U-2: jangan
-- hapus jejak).
--
-- Untuk melihat apakah ada baris seperti itu, jalankan SELECT ini lebih dulu:
--     SELECT mbh_id, mbh_mb_costing, mbh_entry_status,
--            mbh_unlock_requested_at, mbh_unlock_requested_by
--       FROM mst_mb_head
--      WHERE mbh_entry_status = 'UNLOCK_REQUESTED';
--
-- Nama constraint TIDAK berubah oleh rollback ini: 000488 sudah menjadikannya
-- bernama (`chk_mbh_entry_status`), dan down ini memasang ulang nama yang sama
-- dengan isi tujuh-nilai. Skema kembali IDENTIK dengan keadaan pasca-000488.
--
-- ⛔ NOL UPDATE / NOL DELETE. Murni perubahan constraint, seperti up.sql-nya.
-- ⛔ Kolom lock 000485 dan tabel mst_mb_head_lock_log TIDAK disentuh di sini —
--    itu milik down 000485, bukan milik migrasi ini.

BEGIN;

ALTER TABLE mst_mb_head
  DROP CONSTRAINT IF EXISTS chk_mbh_entry_status;

ALTER TABLE mst_mb_head
  ADD CONSTRAINT chk_mbh_entry_status
  CHECK (mbh_entry_status IN (
    'DRAFT', 'SUBMITTED', 'APPROVED', 'VALIDATED', 'UN_APPROVED', 'REVOKED', 'REJECTED'
  ));

COMMENT ON COLUMN mst_mb_head.mbh_entry_status IS
  'New MB Costing workflow state — distinct from legacy mbh_status/mbh_check_status, '
  'never confused with those. Tujuh nilai sah: DRAFT, SUBMITTED, APPROVED, VALIDATED, '
  'UN_APPROVED, REVOKED, REJECTED. REJECTED ditambahkan 000488 untuk keputusan user K-2: '
  'SUBMITTED → REJECTED WAJIB beralasan (alasan disimpan di mbh_state_reason), dan '
  'REJECTED → DRAFT adalah satu-satunya transisi keluar. Penegakan urutan transisi ada '
  'di lapisan domain Go, BUKAN di CHECK ini — CHECK hanya membatasi himpunan nilai.';

COMMIT;
