-- 000581: Seed 8 RM groups (+ their dummy item rows) from Oracle
-- MGTAPPS.CST_GRP_HEAD / CST_GRP_ITEM that the October 2026 new-yarn routing
-- import references but production does not have yet.
--
-- Source: docs/new-yarn-import/08-oracle-missing-rm-groups.sql.txt (read-only
-- trace, 2026-10-22). Pattern mirrors 000460:
--   group_code  = CGH_SYS_ID
--   group_name  = CGH_GROUP_CODE, description = CGH_DESCRIPTION + legacy marker
--   item rows   = CST_GRP_ITEM; all 8 legacy rows are DUMMY (no item code), so
--                 they are inserted as empty-code dummies, inactive (000460 rule).
-- Legacy CGI_FIXVAL_DUTY / CGH_FIXVAL_DUTY (500 / 2000 / 16000) is NOT mapped:
-- cost fields default 0 and are set by the costing team in the RM Group UI /
-- calc engine, as for the 26 groups of 000460.
--
-- Users of these groups (legacy G3): 4661, 8662, 14327, 14328 (202306701);
-- 7520, 14712 (202406788); 14427; 14432; 14433; 14604; 14679 (x2).
--
-- Additive + idempotent: ON CONFLICT DO NOTHING. Marker created_by = 'seed_000581'.
BEGIN;

CREATE TEMP TABLE _stg_rmg_head_579 (
  group_code VARCHAR(30), group_name VARCHAR(200), description TEXT, is_active BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_rmg_head_579 (group_code, group_name, description, is_active) VALUES
('202306701', '250/48 CATIONIC RECYCLE', '250/48 CATIONIC RECYCLE [legacy_code=250/48 CATIONIC RECYCLE]', TRUE),
('202406788', 'POY 125/72 BSD', 'POY 125/72 BSD [legacy_code=POY 125/72 BSD]', TRUE),
('202505853', 'POY 500/144 BR', 'POY 500/144/TBL/BR/SIM/NS/1/O [legacy_code=POY 500/144 BR]', TRUE),
('202608945', 'N66 DTY 70/17/2', 'N66 DTY 70/17/2 SD RW NIM [legacy_code=N66 DTY 70/17/2]', TRUE),
('202608946', 'N66 DTY 70/34/4', 'N66 DTY 70/34/4 SD RW NIM [legacy_code=N66 DTY 70/34/4]', TRUE),
('202609947', 'FDY 75/144 SD SLIT', 'FDY 75/144 SD SLIT [legacy_code=FDY 75/144 SD SLIT]', TRUE),
('202609949', 'FDY 50/36 40% ELONG', 'FDY 50/36/RND/SD-40% ELONGATION [legacy_code=FDY 50/36 40% ELONG]', TRUE),
('202609950', 'POY 80/36 42% SHRINK', 'POY 80/36/RND/SD-42% SHRINKAGE [legacy_code=POY 80/36 42% SHRINK]', TRUE);

INSERT INTO cst_rm_group_head
  (group_code, group_name, description, is_active, created_by)
SELECT s.group_code, s.group_name, s.description, s.is_active, 'seed_000581'
FROM _stg_rmg_head_579 s
ON CONFLICT (group_code) WHERE deleted_at IS NULL DO NOTHING;

CREATE TEMP TABLE _stg_rmg_item_579 (
  group_code VARCHAR(30), item_code VARCHAR(20), grade_code VARCHAR(40),
  is_dummy BOOLEAN, is_active BOOLEAN
) ON COMMIT DROP;

INSERT INTO _stg_rmg_item_579 (group_code, item_code, grade_code, is_dummy, is_active) VALUES
('202306701', '', 'NA', TRUE, FALSE),
('202406788', '', 'NA', TRUE, FALSE),
('202505853', '', 'NA', TRUE, FALSE),
('202608945', '', 'NL', TRUE, FALSE),
('202608946', '', 'NL', TRUE, FALSE),
('202609947', '', 'NA', TRUE, FALSE),
('202609949', '', 'NA', TRUE, FALSE),
('202609950', '', 'NA', TRUE, FALSE);

-- Only for groups seeded by this migration, and only once per group.
INSERT INTO cst_rm_group_detail
  (group_head_id, item_code, grade_code, is_dummy, is_active, created_by)
SELECT h.group_head_id, s.item_code, s.grade_code, s.is_dummy, s.is_active, 'seed_000581'
FROM _stg_rmg_item_579 s
JOIN cst_rm_group_head h ON h.group_code = s.group_code AND h.deleted_at IS NULL
                        AND h.created_by = 'seed_000581'
WHERE NOT EXISTS (
  SELECT 1 FROM cst_rm_group_detail d
  WHERE d.group_head_id = h.group_head_id AND d.created_by = 'seed_000581')
ON CONFLICT DO NOTHING;

DO $$
DECLARE g INTEGER; i INTEGER;
BEGIN
  SELECT count(*) INTO g FROM cst_rm_group_head WHERE created_by = 'seed_000581';
  SELECT count(*) INTO i FROM cst_rm_group_detail WHERE created_by = 'seed_000581';
  RAISE NOTICE '000581: rm_group heads=% (expect 8)  dummy items=% (expect 8)', g, i;
END $$;

COMMIT;
