-- 000485: penguncian MB recipe (area E) — kolom lock pada mst_mb_head
--         + tabel audit sejajar mst_mb_head_lock_log.
--
-- 🔴 PENYIMPANGAN DISENGAJA DARI DESIGN §2.12 — DICATAT, BUKAN KELALAIAN:
-- design menulis  mbh_is_locked BOOLEAN NOT NULL DEFAULT FALSE.
-- Di sini kolom itu dibuat NULLable TANPA DEFAULT, mengikuti aturan keras
-- fase P3: setiap kolom baru pada tabel EKSISTING wajib nullable, tanpa
-- DEFAULT, tanpa NOT NULL, supaya data impor Oracle tidak tersentuh (U-2).
-- ⇒ KONSEKUENSI untuk lapisan Go (fase P10): mbh_is_locked bernilai NULL pada
--    SELURUH baris lama. NULL WAJIB diperlakukan sebagai "tidak terkunci".
--    Pakai COALESCE(mbh_is_locked, FALSE) di setiap pembacaan, dan ⛔ JANGAN
--    menulis "WHERE mbh_is_locked = FALSE" — predikat itu melewatkan NULL.
--    Bila kelak diinginkan NOT NULL + DEFAULT FALSE, itu backfill = migrasi
--    tersendiri di fase BERISIKO, bukan di sini.
--
-- Tabel mst_mb_head_lock_log adalah tabel BARU ⇒ NOT NULL/CHECK di dalamnya
-- tidak menyentuh baris lama mana pun.
--
-- Mengapa tabel SEJAJAR, bukan menumpang mst_mb_lock_log (000449):
-- mst_mb_lock_log.mbll_cost_product_id ber-FK ke cost_product_master dan
-- bertipe BIGINT, sedangkan head ber-PK UUID. Menumpang memaksa kolom
-- polymorphic tanpa FK. Dua objek berbeda — ⛔ jangan digabung.
--
-- ⛔ NOL UPDATE / NOL backfill / NOL seed.

BEGIN;

ALTER TABLE mst_mb_head
  ADD COLUMN IF NOT EXISTS mbh_is_locked           BOOLEAN,
  ADD COLUMN IF NOT EXISTS mbh_locked_at           TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS mbh_locked_by           VARCHAR(20),
  ADD COLUMN IF NOT EXISTS mbh_unlock_requested_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS mbh_unlock_requested_by VARCHAR(20),
  ADD COLUMN IF NOT EXISTS mbh_unlock_reason       TEXT;

COMMENT ON COLUMN mst_mb_head.mbh_is_locked IS
  'Recipe terkunci? NULL = belum pernah disentuh mekanisme lock = TIDAK terkunci. '
  'Baca dengan COALESCE(mbh_is_locked, FALSE); jangan bandingkan langsung dengan FALSE.';
COMMENT ON COLUMN mst_mb_head.mbh_unlock_requested_at IS
  'Kapan permintaan unlock diajukan. NULL = tidak ada permintaan tertunda.';
COMMENT ON COLUMN mst_mb_head.mbh_unlock_requested_by IS
  'Pengaju permintaan unlock. NULL = tidak ada permintaan tertunda.';

CREATE TABLE IF NOT EXISTS mst_mb_head_lock_log (
  mbhl_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbhl_mbh_id          UUID        NOT NULL,
  mbhl_event           VARCHAR(20) NOT NULL
                       CHECK (mbhl_event IN ('LOCK','UNLOCK_REQUEST','UNLOCK_GRANT','UNLOCK_REJECT','RELOCK')),
  mbhl_actor_user_id   VARCHAR(20) NOT NULL,
  mbhl_actor_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  mbhl_reason          TEXT,
  mbhl_auto_relock_at  TIMESTAMPTZ,
  mbhl_meta            JSONB,
  CONSTRAINT fk_mbhl_mbh FOREIGN KEY (mbhl_mbh_id)
    REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mbhl_mbh_at
  ON mst_mb_head_lock_log (mbhl_mbh_id, mbhl_actor_at DESC);

CREATE INDEX IF NOT EXISTS idx_mbhl_auto_relock
  ON mst_mb_head_lock_log (mbhl_auto_relock_at) WHERE mbhl_auto_relock_at IS NOT NULL;

COMMENT ON TABLE mst_mb_head_lock_log IS
  'Audit lock/unlock resep MB (area E). Sejajar dengan mst_mb_lock_log (000449) yang '
  'mengunci cost_product_master — dua objek berbeda, jangan digabung. Auto-relock 24 jam '
  'dijejaki lewat mbhl_auto_relock_at (konsisten dengan R9).';

COMMIT;
