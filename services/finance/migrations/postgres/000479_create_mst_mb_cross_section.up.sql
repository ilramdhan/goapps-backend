-- 000479: MB Costing Suite — cross-section lookup master.
--
-- Follows the mst_mb_lusture pattern (000441): a small, user-editable code
-- master with display name / description / display order, audit columns and
-- soft delete. Design reference: consolidated MB recipe design §2.7 (the design
-- numbered this migration 000482; the applied number is 000479 because the
-- preceding design migrations are not part of this batch).
--
-- mbh_cross_section is deliberately NOT given a FK to this table: that column is
-- VARCHAR(20) while mbcs_code is VARCHAR(10), and legacy rows are NULL/off-domain.
-- Consistency is enforced by the planned chk_mbh_cross_section CHECK (a later
-- migration in this feature, not yet applied) plus the Go domain, mirroring the
-- existing soft-link used for mbh_lusture_code.

BEGIN;

CREATE TABLE IF NOT EXISTS mst_mb_cross_section (
  mbcs_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  mbcs_code           VARCHAR(10)  NOT NULL UNIQUE,
  mbcs_display_name   VARCHAR(50),
  mbcs_description    VARCHAR(200),
  mbcs_is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
  mbcs_display_order  INTEGER,
  mbcs_created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  mbcs_created_by     VARCHAR(20)  NOT NULL,
  mbcs_updated_at     TIMESTAMPTZ,
  mbcs_updated_by     VARCHAR(20),
  deleted_at          TIMESTAMPTZ,
  deleted_by          VARCHAR(20)
);

CREATE INDEX IF NOT EXISTS idx_mbcs_active_order
  ON mst_mb_cross_section (mbcs_display_order) WHERE mbcs_is_active = TRUE;

COMMENT ON TABLE mst_mb_cross_section IS
  'MB Costing Suite cross-section lookup master — 6 seeded rows (see migration 000479). '
  'User-editable via the Finance master UI.';

-- ============================================================
-- SEED — 6 rows, matching the 6 values of the planned chk_mbh_cross_section.
-- ============================================================
-- The count is 6, not 5. The CHECK constraint that a later migration will put on
-- mst_mb_head.mbh_cross_section admits 6 values. Seeding only 5 here would leave
-- the master and that CHECK silently out of sync (an MB head could hold a code
-- with no master row behind it). If either list ever changes, change both.
--
-- mbcs_display_order is a presentation convention only (order of appearance
-- below). It carries no business meaning and may be reordered freely through
-- the master UI.
--
-- Idempotent: one guarded INSERT per row (pattern of 000474 / 000475 / 000476),
-- so a re-run is a no-op and a soft-deleted row is not resurrected by a plain
-- ON CONFLICT.

INSERT INTO mst_mb_cross_section (mbcs_code, mbcs_display_name, mbcs_description, mbcs_display_order, mbcs_created_by)
SELECT 'RND', 'Round', NULL, 1, 'seed_000479'
WHERE NOT EXISTS (SELECT 1 FROM mst_mb_cross_section WHERE mbcs_code = 'RND' AND deleted_at IS NULL);

INSERT INTO mst_mb_cross_section (mbcs_code, mbcs_display_name, mbcs_description, mbcs_display_order, mbcs_created_by)
SELECT 'TBL', 'Trilobal', NULL, 2, 'seed_000479'
WHERE NOT EXISTS (SELECT 1 FROM mst_mb_cross_section WHERE mbcs_code = 'TBL' AND deleted_at IS NULL);

INSERT INTO mst_mb_cross_section (mbcs_code, mbcs_display_name, mbcs_description, mbcs_display_order, mbcs_created_by)
SELECT 'OTL', 'Octalobal', NULL, 3, 'seed_000479'
WHERE NOT EXISTS (SELECT 1 FROM mst_mb_cross_section WHERE mbcs_code = 'OTL' AND deleted_at IS NULL);

INSERT INTO mst_mb_cross_section (mbcs_code, mbcs_display_name, mbcs_description, mbcs_display_order, mbcs_created_by)
SELECT 'SPC', 'Special', NULL, 4, 'seed_000479'
WHERE NOT EXISTS (SELECT 1 FROM mst_mb_cross_section WHERE mbcs_code = 'SPC' AND deleted_at IS NULL);

INSERT INTO mst_mb_cross_section (mbcs_code, mbcs_display_name, mbcs_description, mbcs_display_order, mbcs_created_by)
SELECT 'PLUS', 'Plus', NULL, 5, 'seed_000479'
WHERE NOT EXISTS (SELECT 1 FROM mst_mb_cross_section WHERE mbcs_code = 'PLUS' AND deleted_at IS NULL);

-- RSD: display name/description belum ditentukan (tidak ada di plan/design). Diisi = kode; DIHARAPKAN diperbarui lewat UI master setelah Finance menetapkannya. Jangan tebak kepanjangannya.
INSERT INTO mst_mb_cross_section (mbcs_code, mbcs_display_name, mbcs_description, mbcs_display_order, mbcs_created_by)
SELECT 'RSD', 'RSD', NULL, 6, 'seed_000479'
WHERE NOT EXISTS (SELECT 1 FROM mst_mb_cross_section WHERE mbcs_code = 'RSD' AND deleted_at IS NULL);

COMMIT;
