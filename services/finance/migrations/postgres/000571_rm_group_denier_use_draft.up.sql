-- 000571 (rm-group-denier-draft-flag) — per RM group flag deciding whether the
-- group denier is divided by the product's SPANDEX_DRAFT in DENIER ratio mode.
--
-- Finance decision (2026-10-14): only spandex is divided by the draft; bought-out
-- POY/FDY groups use their denier as-is. Before this flag the engine divided
-- EVERY group denier by SPANDEX_DRAFT.
--
-- 1. cst_rm_group_head.denier_use_draft BOOLEAN NOT NULL DEFAULT FALSE.
--    Head-level and global (not period-versioned), like denier (000570).
-- 2. Spandex groups (group_name LIKE 'SPD%', SPD0000001..010) seeded TRUE so
--    existing ACY results do not change.
--
-- Idempotent: ADD COLUMN IF NOT EXISTS, guarded UPDATE.

BEGIN;

SET LOCAL lock_timeout = '10s';

ALTER TABLE cst_rm_group_head
    ADD COLUMN IF NOT EXISTS denier_use_draft BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN cst_rm_group_head.denier_use_draft IS
    'When TRUE the group denier is divided by the product SPANDEX_DRAFT in DENIER ratio mode (spandex). FALSE: denier used as-is (bought-out POY/FDY). Head-level/global, not period-versioned.';

UPDATE cst_rm_group_head
SET denier_use_draft = TRUE
WHERE group_name LIKE 'SPD%'
  AND denier_use_draft = FALSE;

COMMIT;
