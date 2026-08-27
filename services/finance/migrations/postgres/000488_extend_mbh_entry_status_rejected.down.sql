-- Down 000488: kembalikan CHECK `mbh_entry_status` ke enam nilai (tanpa 'REJECTED').
--
-- ⚠ FAKTA YANG DICATAT JUJUR — NAMA CONSTRAINT BERUBAH DIBANDING PRA-000488:
-- sebelum 000488 constraint itu ANONIM (auto-generate PostgreSQL, dibuat oleh
-- CHECK inline di 000445:5-7). Bentuk anonim ⛔ TIDAK DAPAT dipulihkan secara
-- deterministik: PostgreSQL memilih sendiri nama auto-generate, dan menambahkan
-- CHECK inline lewat ALTER TABLE tetap menghasilkan nama pilihan server yang
-- belum tentu identik dengan yang dulu. Karena itu down ini mengembalikan ISI
-- constraint (enam nilai) sebagai constraint BERNAMA `chk_mbh_entry_status`.
-- ⇒ Setelah rollback, skema SETARA secara SEMANTIK dengan kondisi pra-000488,
--    tetapi NAMA constraint-nya berbeda (bernama, bukan anonim). Ini disengaja
--    dan dicatat — bukan kelalaian. Nama bernama justru lebih baik: migrasi
--    berikutnya tidak perlu lagi menebak nama.
--
-- ⚠ ROLLBACK INI AKAN GAGAL — SENGAJA — BILA SUDAH ADA BARIS BERSTATUS 'REJECTED'.
-- PostgreSQL memvalidasi CHECK baru terhadap seluruh baris; baris 'REJECTED'
-- akan menolak ADD CONSTRAINT dan seluruh transaksi ini dibatalkan. Itu perilaku
-- yang BENAR: menyempitkan enum saat datanya masih memakai nilai yang dibuang
-- berarti data harus diputuskan lebih dulu OLEH USER (mau dikembalikan ke DRAFT?
-- ke UN_APPROVED?). ⛔ Down ini SENGAJA TIDAK meng-UPDATE baris apa pun —
-- migrasi tidak boleh diam-diam mengubah status kerja orang.
-- Untuk melihat apakah ada baris seperti itu, jalankan SELECT ini lebih dulu:
--     SELECT mbh_id, mbh_entry_status FROM mst_mb_head WHERE mbh_entry_status = 'REJECTED';
--
-- ⛔ NOL UPDATE / NOL DELETE. Murni perubahan constraint, seperti up.sql-nya.

BEGIN;

ALTER TABLE mst_mb_head
  DROP CONSTRAINT IF EXISTS chk_mbh_entry_status;

ALTER TABLE mst_mb_head
  ADD CONSTRAINT chk_mbh_entry_status
  CHECK (mbh_entry_status IN (
    'DRAFT', 'SUBMITTED', 'APPROVED', 'VALIDATED', 'UN_APPROVED', 'REVOKED'
  ));

COMMENT ON COLUMN mst_mb_head.mbh_entry_status IS
  'New MB Costing workflow state — distinct from legacy mbh_status/mbh_check_status, '
  'never confused with those';

COMMIT;
