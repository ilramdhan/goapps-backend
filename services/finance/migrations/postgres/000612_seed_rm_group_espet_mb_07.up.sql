-- 000612: Seed RM group 202611954 'ESPET-MB-07' (EASY-DYEABLE MASTERBATCH, + its dummy item)
-- from Oracle MGTAPPS.CST_GRP_HEAD / CST_GRP_ITEM. It is the only composition row (100 %) of the
-- new MB recipe 20261105580 (ESPET-MB-07, mb-active-sync NEW_RECIPE) and does not exist in any env.
--
-- Source: docs/mb-active-sync/oracle-results/O12-rm-group.txt (read-only trace, 2026-11-20).
-- Pattern = 000581 / 000460: group_code = CGH_SYS_ID, group_name = CGH_GROUP_CODE,
-- description = CGH_DESCRIPTION + legacy marker. The legacy item row is DUMMY (no item code,
-- grade NA) -> empty-code dummy, inactive. Legacy CGH_FIXVAL_DUTY 500 is NOT mapped (cost fields
-- default 0, set by the costing team in the RM Group UI, as for 000581). The real ERP item
-- (MBC0000307 Masterbatch Easy Dyeable TA10966-A1) comes from the ERP item sync per env and is
-- attached in the RM Group UI, not here.
--
-- Additive + idempotent: ON CONFLICT DO NOTHING. Marker created_by = 'seed_000612'.
BEGIN;

CREATE TEMP TABLE _stg_rmg_head_612 (
  group_code VARCHAR(30), group_name VARCHAR(200), description TEXT, is_active BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_rmg_head_612 (group_code, group_name, description, is_active) VALUES
('202611954', 'ESPET-MB-07', 'EASY-DYEABLE MASTERBATCH [legacy_code=ESPET-MB-07]', TRUE);

INSERT INTO cst_rm_group_head
  (group_code, group_name, description, is_active, created_by)
SELECT s.group_code, s.group_name, s.description, s.is_active, 'seed_000612'
FROM _stg_rmg_head_612 s
ON CONFLICT (group_code) WHERE deleted_at IS NULL DO NOTHING;

CREATE TEMP TABLE _stg_rmg_item_612 (
  group_code VARCHAR(30), item_code VARCHAR(20), grade_code VARCHAR(40),
  is_dummy BOOLEAN, is_active BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_rmg_item_612 (group_code, item_code, grade_code, is_dummy, is_active) VALUES
('202611954', '', 'NA', TRUE, FALSE);

-- Only for groups seeded by this migration, and only once per group.
INSERT INTO cst_rm_group_detail
  (group_head_id, item_code, grade_code, is_dummy, is_active, created_by)
SELECT h.group_head_id, s.item_code, s.grade_code, s.is_dummy, s.is_active, 'seed_000612'
FROM _stg_rmg_item_612 s
JOIN cst_rm_group_head h ON h.group_code = s.group_code AND h.deleted_at IS NULL
                        AND h.created_by = 'seed_000612'
WHERE NOT EXISTS (
  SELECT 1 FROM cst_rm_group_detail d
  WHERE d.group_head_id = h.group_head_id AND d.created_by = 'seed_000612')
ON CONFLICT DO NOTHING;

DO $$
DECLARE g INTEGER; i INTEGER;
BEGIN
  SELECT count(*) INTO g FROM cst_rm_group_head WHERE created_by = 'seed_000612';
  SELECT count(*) INTO i FROM cst_rm_group_detail WHERE created_by = 'seed_000612';
  RAISE NOTICE '000612: rm_group heads=% (expect 1)  dummy items=% (expect 1)', g, i;
END $$;

COMMIT;
