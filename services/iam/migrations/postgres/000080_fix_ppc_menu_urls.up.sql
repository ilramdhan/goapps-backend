-- Fix PPC menu URLs to match the actual Next.js route folders.
--
-- Migration 000079 seeded singular menu_url values (e.g. /masters/machine)
-- but the frontend route folders are plural (e.g. /masters/machines). The
-- mismatch made every affected sidebar link resolve to a 404. This migration
-- realigns the 8 mismatched menu_url values with the real routes. Keyed by
-- menu_code (stable) rather than menu_id. Idempotent (plain UPDATE).

UPDATE mst_menu SET menu_url = '/production-plan/dashboard'
  WHERE menu_code = 'PPC_DASHBOARDS';

UPDATE mst_menu SET menu_url = '/production-plan/masters/machines'
  WHERE menu_code = 'PPC_MASTER_MACHINE';

UPDATE mst_menu SET menu_url = '/production-plan/masters/machine-groups'
  WHERE menu_code = 'PPC_MASTER_MACHINE_GROUP';

UPDATE mst_menu SET menu_url = '/production-plan/masters/lots'
  WHERE menu_code = 'PPC_MASTER_LOT';

UPDATE mst_menu SET menu_url = '/production-plan/masters/product-machine-parameters'
  WHERE menu_code = 'PPC_MASTER_MACHINE_PARAMETER';

UPDATE mst_menu SET menu_url = '/production-plan/masters/thresholds'
  WHERE menu_code = 'PPC_MASTER_THRESHOLD';

UPDATE mst_menu SET menu_url = '/production-plan/masters/downtime-reasons'
  WHERE menu_code = 'PPC_MASTER_DOWNTIME_REASON';

UPDATE mst_menu SET menu_url = '/production-plan/masters/waste-categories'
  WHERE menu_code = 'PPC_MASTER_WASTE_CATEGORY';
