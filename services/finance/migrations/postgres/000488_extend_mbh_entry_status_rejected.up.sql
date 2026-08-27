-- 000488: melebarkan CHECK `mbh_entry_status` pada mst_mb_head dengan nilai
--         KE-7 `'REJECTED'`.
--
-- ⭐ DASAR KEPUTUSAN USER — ⛔ JANGAN diubah tanpa keputusan user baru:
--   * `K-2` — transisi `SUBMITTED → REJECTED` WAJIB beralasan (alasan ditulis ke
--     `mbh_state_reason`, kolom yang sudah ada sejak 000445), dan dari REJECTED
--     satu-satunya jalan keluar adalah `REJECTED → DRAFT`.
--   ⇒ Konsekuensi skema: enam nilai yang dipasang 000445 tidak lagi cukup;
--     CHECK harus memuat TUJUH nilai.
--
-- ⚠ RANTAI RENUMBERING MASIH TERBUKA — plan §11 butir 126 (`K-15`): rencana
-- renumbering migrasi finance BELUM dieksekusi dan MASIH TERBUKA. Bila rantai itu
-- kelak dijalankan, nomor `000488` di sini IKUT BERGESER. Nomor ini dipilih
-- karena bebas (tertinggi di disk saat penulisan = 000487), bukan karena ia
-- final. ⛔ Jangan jadikan angka 488 sebagai acuan keras di kode/dokumen lain.
--
-- ⛔ MURNI PERUBAHAN CONSTRAINT. NOL UPDATE, NOL backfill, NOL seed, NOL kolom
-- baru. Tidak satu baris pun ditulis ulang; `updated_at` tidak bergerak.
-- Melebarkan CHECK bersifat aditif: setiap baris yang lolos CHECK lama pasti
-- lolos CHECK baru, jadi validasi ulang tabel oleh PostgreSQL tidak akan gagal.
--
-- ============================================================================
-- 🔴 MENGAPA ADA BLOK DO $$ DI BAWAH — BACA SEBELUM MENYEDERHANAKAN
-- ============================================================================
-- 000445:5-7 membuat kolom ini dengan CHECK INLINE TANPA NAMA:
--     mbh_entry_status VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
--       CHECK (mbh_entry_status IN (...6 nilai...))
-- Nama constraint-nya karena itu DI-AUTO-GENERATE PostgreSQL. Lazimnya
-- `mst_mb_head_mbh_entry_status_check`, tetapi itu ⛔ TIDAK DIJAMIN — bila nama
-- itu sudah terpakai, PostgreSQL menambah sufiks (`..._check1`, `..._check2`).
--
-- Karena itu `DROP CONSTRAINT IF EXISTS <nama-tebakan>` BERBAHAYA di sini:
-- bila tebakan meleset, DROP itu DIAM-DIAM NO-OP, lalu ADD di bawah memasang
-- CHECK KEDUA. Kedua CHECK berlaku BERSAMAAN (AND), sehingga CHECK enam-nilai
-- yang lama TETAP MENOLAK `'REJECTED'` — dan migrasi tetap SUKSES TANPA ERROR
-- APA PUN. Itu kegagalan SENYAP yang baru ketahuan saat user menekan Reject.
--
-- Blok DO di bawah menghilangkan tebakan: ia MENEMUKAN nama sebenarnya dari
-- katalog (`pg_constraint` × `pg_attribute`, dicocokkan lewat KOLOM `conkey`,
-- bukan lewat pencocokan teks nama), lalu men-DROP-nya via EXECUTE format(%I).
-- Bila NOL constraint ditemukan ⇒ RAISE EXCEPTION: gagal NYARING, bukan senyap.

BEGIN;

DO $$
DECLARE
  v_attnum smallint;
  v_name   text;
  v_count  integer := 0;
BEGIN
  SELECT a.attnum
    INTO v_attnum
    FROM pg_attribute a
    JOIN pg_class c ON c.oid = a.attrelid
   WHERE c.relname = 'mst_mb_head'
     AND c.relkind IN ('r', 'p')
     AND a.attname = 'mbh_entry_status'
     AND a.attnum > 0
     AND NOT a.attisdropped;

  IF v_attnum IS NULL THEN
    RAISE EXCEPTION
      'Migrasi 000488 DIBATALKAN: kolom mst_mb_head.mbh_entry_status tidak ditemukan. '
      'Kolom itu seharusnya dibuat oleh 000445. Periksa urutan migrasi sebelum melanjutkan.';
  END IF;

  -- Semua CHECK constraint pada mst_mb_head yang MENYENTUH kolom ini, apa pun
  -- namanya. Dicocokkan lewat conkey (identitas kolom), sehingga sufiks nama
  -- auto-generate (_check / _check1 / _check2) tidak lagi relevan.
  FOR v_name IN
    SELECT con.conname
      FROM pg_constraint con
      JOIN pg_class c ON c.oid = con.conrelid
     WHERE c.relname = 'mst_mb_head'
       AND con.contype = 'c'
       AND v_attnum = ANY (con.conkey)
       AND pg_get_constraintdef(con.oid) LIKE '%mbh_entry_status%'
  LOOP
    RAISE NOTICE 'Migrasi 000488: men-DROP CHECK lama pada mbh_entry_status: %', v_name;
    EXECUTE format('ALTER TABLE mst_mb_head DROP CONSTRAINT %I', v_name);
    v_count := v_count + 1;
  END LOOP;

  IF v_count = 0 THEN
    RAISE EXCEPTION
      'Migrasi 000488 DIBATALKAN: TIDAK ADA satu pun CHECK constraint pada '
      'mst_mb_head.mbh_entry_status. Yang diharapkan: CHECK enam-nilai anonim dari '
      '000445:5-7. Melanjutkan tanpa itu berarti keadaan skema berbeda dari asumsi '
      'migrasi ini — SELIDIKI DULU (pg_constraint), jangan paksa jalan.';
  END IF;
END
$$;

-- Nama EKSPLISIT, mengikuti konvensi 000487 (`chk_mbh_check_status_calc`):
-- prefiks `chk_` + nama kolom. Dengan nama sendiri, migrasi berikutnya yang perlu
-- melebarkan enum ini cukup memakai DROP ... IF EXISTS biasa — tanpa blok DO,
-- tanpa tebak-tebakan nama.
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
