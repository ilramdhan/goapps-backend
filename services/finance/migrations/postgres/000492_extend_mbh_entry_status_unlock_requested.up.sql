-- 000492: melebarkan CHECK `mbh_entry_status` pada mst_mb_head dengan nilai
--         KE-8 `'UNLOCK_REQUESTED'`.
--
-- ⭐ DASAR — P10 (lock/unlock resep MB Head). Kolom lock sudah dipasang oleh
--   000485 (`mbh_is_locked`, `mbh_locked_at/by`, `mbh_unlock_requested_at/by`,
--   `mbh_unlock_reason`) beserta tabel audit `mst_mb_head_lock_log`. Yang belum
--   ada adalah STATE tempat resep menunggu keputusan unlock. P10 menambah state
--   itu: `APPROVED → UNLOCK_REQUESTED`, `VALIDATED → UNLOCK_REQUESTED`, lalu
--   keluar lewat `UNLOCK_REQUESTED → {APPROVED, VALIDATED, DRAFT}`.
--   ⇒ Konsekuensi skema: tujuh nilai yang dipasang 000488 tidak lagi cukup;
--     CHECK harus memuat DELAPAN nilai.
--
-- 🔴 PERLUASAN, BUKAN PENGGANTIAN. Tidak ada satu pun nilai yang dicabut —
-- ketujuh nilai lama tetap sah. 4190 baris berstatus `VALIDATED` yang sudah ada
-- di produksi TETAP LOLOS CHECK baru ini. Melebarkan CHECK bersifat aditif:
-- setiap baris yang lolos CHECK lama pasti lolos CHECK baru, jadi validasi ulang
-- tabel oleh PostgreSQL tidak akan gagal.
--
-- ⛔ MURNI PERUBAHAN CONSTRAINT. NOL UPDATE, NOL backfill, NOL seed, NOL kolom
-- baru. Tidak satu baris pun ditulis ulang; `updated_at` tidak bergerak.
--
-- ⚠ RANTAI RENUMBERING MASIH TERBUKA — sama seperti catatan di 000488 (`K-15`):
-- rencana renumbering migrasi finance BELUM dieksekusi dan MASIH TERBUKA. Bila
-- rantai itu kelak dijalankan, nomor `000492` di sini IKUT BERGESER. Nomor ini
-- dipilih karena bebas (tertinggi di disk saat penulisan = 000491), bukan karena
-- ia final. ⛔ Jangan jadikan angka 492 sebagai acuan keras di kode/dokumen lain.
--
-- ============================================================================
-- 🔴 MENGAPA DI SINI CUKUP `DROP CONSTRAINT IF EXISTS` — TANPA BLOK DO
-- ============================================================================
-- 000488 sudah menyelesaikan masalah nama anonim: ia menemukan CHECK anonim
-- bikinan 000445 lewat katalog, men-DROP-nya, lalu memasang ulang dengan nama
-- EKSPLISIT `chk_mbh_entry_status` (konvensi `chk_` + nama kolom). Karena nama
-- itu sekarang DIKETAHUI dan DITETAPKAN oleh migrasi, tidak ada lagi yang perlu
-- ditebak — `DROP CONSTRAINT IF EXISTS chk_mbh_entry_status` tidak mungkin
-- salah sasaran. Itulah alasan 000488 memberi nama eksplisit; blok DO di sana
-- adalah pembayaran satu kali, bukan pola yang harus diulang.
--
-- Namun `IF EXISTS` yang no-op senyap tetap berbahaya (akan menghasilkan CHECK
-- KEDUA yang berlaku ber-AND dan tetap menolak 'UNLOCK_REQUESTED' TANPA error).
-- Karena itu keberadaan constraint itu DIPASTIKAN DULU lewat blok verifikasi di
-- bawah: bila `chk_mbh_entry_status` tidak ada, migrasi GAGAL NYARING.

BEGIN;

DO $$
DECLARE
  v_exists boolean;
BEGIN
  SELECT EXISTS (
    SELECT 1
      FROM pg_constraint con
      JOIN pg_class c ON c.oid = con.conrelid
     WHERE c.relname = 'mst_mb_head'
       AND con.contype = 'c'
       AND con.conname = 'chk_mbh_entry_status'
  ) INTO v_exists;

  IF NOT v_exists THEN
    RAISE EXCEPTION
      'Migrasi 000492 DIBATALKAN: constraint chk_mbh_entry_status TIDAK DITEMUKAN pada '
      'mst_mb_head. Yang diharapkan: CHECK tujuh-nilai BERNAMA yang dipasang 000488. '
      'Keadaan skema berbeda dari asumsi migrasi ini — SELIDIKI DULU (pg_constraint), '
      'jangan paksa jalan. Bila constraint-nya masih ANONIM, berarti 000488 belum '
      'dijalankan: jalankan 000488 lebih dulu.';
  END IF;
END
$$;

ALTER TABLE mst_mb_head
  DROP CONSTRAINT IF EXISTS chk_mbh_entry_status;

ALTER TABLE mst_mb_head
  ADD CONSTRAINT chk_mbh_entry_status
  CHECK (mbh_entry_status IN (
    'DRAFT', 'SUBMITTED', 'APPROVED', 'VALIDATED', 'UN_APPROVED', 'REVOKED', 'REJECTED',
    'UNLOCK_REQUESTED'
  ));

COMMENT ON COLUMN mst_mb_head.mbh_entry_status IS
  'New MB Costing workflow state — distinct from legacy mbh_status/mbh_check_status, '
  'never confused with those. Delapan nilai sah: DRAFT, SUBMITTED, APPROVED, VALIDATED, '
  'UN_APPROVED, REVOKED, REJECTED, UNLOCK_REQUESTED. REJECTED ditambahkan 000488 untuk '
  'keputusan user K-2: SUBMITTED → REJECTED WAJIB beralasan (alasan disimpan di '
  'mbh_state_reason), dan REJECTED → DRAFT adalah satu-satunya transisi keluar. '
  'UNLOCK_REQUESTED ditambahkan 000492 untuk P10 (lock/unlock resep): resep yang sudah '
  'APPROVED/VALIDATED terkunci, dan permintaan unlock memarkir head di UNLOCK_REQUESTED '
  'sampai diputuskan — grant membawanya ke DRAFT (dapat diedit lagi), reject '
  'mengembalikannya ke state asal (APPROVED atau VALIDATED, dibaca dari '
  'mst_mb_workflow_log). Jejak setiap aksi lock/unlock ada di mst_mb_head_lock_log '
  '(000485). Penegakan urutan transisi ada di lapisan domain Go, BUKAN di CHECK ini — '
  'CHECK hanya membatasi himpunan nilai.';

COMMIT;
