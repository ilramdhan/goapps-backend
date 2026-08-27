-- 000483: tabel anak shade tambahan untuk MB recipe.
--
-- Multi-shade (D1): 1 shade ada di header (mbh_shade_code / mbh_shade_name,
-- 000445) + MAKS 2 baris di tabel ini = total 3 shade per head.
--
-- Batas "maks 2" ditegakkan DUA lapis di DB:
--   (1) CHECK (mbhs_seq_no IN (1,2))  -> seq ke-3 mustahil
--   (2) uix_mbhs_mbh_seq              -> satu baris hidup per (head, seq)
-- Domain Go menolak lebih dulu dengan pesan manusiawi (ErrTooManyShades, P5).
--
-- Tabel BARU ⇒ NOT NULL / CHECK diperbolehkan: tidak ada baris lama yang
-- tersentuh. Aturan "wajib nullable" fase P3 berlaku untuk kolom baru pada
-- tabel EKSISTING, bukan untuk tabel yang lahir kosong.
--
-- Kolom audit mengikuti konvensi 000479/000480: <prefix>_created_at/by,
-- <prefix>_updated_at/by, lalu deleted_at/deleted_by TANPA prefix.
--
-- ⛔ Tanpa seed, tanpa UPDATE, tanpa backfill.

BEGIN;

CREATE TABLE IF NOT EXISTS mst_mb_head_shade (
  mbhs_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbhs_mbh_id       UUID         NOT NULL,
  mbhs_seq_no       SMALLINT     NOT NULL CHECK (mbhs_seq_no IN (1, 2)),
  mbhs_shade_code   VARCHAR(20)  NOT NULL,
  mbhs_shade_name   VARCHAR(100),
  mbhs_created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  mbhs_created_by   VARCHAR(20)  NOT NULL,
  mbhs_updated_at   TIMESTAMPTZ,
  mbhs_updated_by   VARCHAR(20),
  deleted_at        TIMESTAMPTZ,
  deleted_by        VARCHAR(20),
  CONSTRAINT fk_mbhs_mbh FOREIGN KEY (mbhs_mbh_id)
    REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE
);

-- Partial unique: satu baris HIDUP per (head, seq). Baris soft-deleted tidak
-- boleh memblokir penggantinya (penalaran sama dengan uq_msfc_period_live
-- di 000474 dan uix_mbcf_pair di 000480).
CREATE UNIQUE INDEX IF NOT EXISTS uix_mbhs_mbh_seq
  ON mst_mb_head_shade (mbhs_mbh_id, mbhs_seq_no) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mbhs_mbh
  ON mst_mb_head_shade (mbhs_mbh_id);

COMMENT ON TABLE mst_mb_head_shade IS
  'Shade tambahan untuk satu MB recipe — maks 2 baris (seq 1..2). Shade ke-3 adalah '
  'pasangan header mbh_shade_code/mbh_shade_name pada mst_mb_head.';

COMMIT;
