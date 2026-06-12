-- Migration 000052: Assign CPR test users to department sections
--
-- Finance users (finance01, financemgr) → FIN department section
-- Production users (production01-03, productionmgr) → PROD department section
-- Marketing users (marketing01, marketingmgr) → MKT department section
--
-- Uses deterministic section lookup by department_code to survive fresh DB setups.

UPDATE mst_user_detail ud
SET section_id = (
  SELECT s.section_id FROM mst_section s
  JOIN mst_department d ON s.department_id = d.department_id
  WHERE d.department_code = 'FIN' AND s.deleted_at IS NULL AND d.deleted_at IS NULL
  ORDER BY s.created_at LIMIT 1
)
WHERE ud.user_id IN (
  SELECT user_id FROM mst_user
  WHERE username IN ('finance01', 'financemgr') AND deleted_at IS NULL
)
AND ud.section_id IS NULL;

UPDATE mst_user_detail ud
SET section_id = (
  SELECT s.section_id FROM mst_section s
  JOIN mst_department d ON s.department_id = d.department_id
  WHERE d.department_code = 'PROD' AND s.deleted_at IS NULL AND d.deleted_at IS NULL
  ORDER BY s.created_at LIMIT 1
)
WHERE ud.user_id IN (
  SELECT user_id FROM mst_user
  WHERE username IN ('production01', 'production02', 'production03', 'productionmgr') AND deleted_at IS NULL
)
AND ud.section_id IS NULL;

UPDATE mst_user_detail ud
SET section_id = (
  SELECT s.section_id FROM mst_section s
  JOIN mst_department d ON s.department_id = d.department_id
  WHERE d.department_code = 'MKT' AND s.deleted_at IS NULL AND d.deleted_at IS NULL
  ORDER BY s.created_at LIMIT 1
)
WHERE ud.user_id IN (
  SELECT user_id FROM mst_user
  WHERE username IN ('marketing01', 'marketingmgr') AND deleted_at IS NULL
)
AND ud.section_id IS NULL;
