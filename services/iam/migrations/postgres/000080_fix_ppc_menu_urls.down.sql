-- Revert PPC menu URLs to the (broken) singular values seeded by 000079.

UPDATE mst_menu SET menu_url = '/production-plan/dashboards'
  WHERE menu_code = 'PPC_DASHBOARDS';

UPDATE mst_menu SET menu_url = '/production-plan/masters/machine'
  WHERE menu_code = 'PPC_MASTER_MACHINE';

UPDATE mst_menu SET menu_url = '/production-plan/masters/machine-group'
  WHERE menu_code = 'PPC_MASTER_MACHINE_GROUP';

UPDATE mst_menu SET menu_url = '/production-plan/masters/lot'
  WHERE menu_code = 'PPC_MASTER_LOT';

UPDATE mst_menu SET menu_url = '/production-plan/masters/machine-parameter'
  WHERE menu_code = 'PPC_MASTER_MACHINE_PARAMETER';

UPDATE mst_menu SET menu_url = '/production-plan/masters/threshold'
  WHERE menu_code = 'PPC_MASTER_THRESHOLD';

UPDATE mst_menu SET menu_url = '/production-plan/masters/downtime-reason'
  WHERE menu_code = 'PPC_MASTER_DOWNTIME_REASON';

UPDATE mst_menu SET menu_url = '/production-plan/masters/waste-category'
  WHERE menu_code = 'PPC_MASTER_WASTE_CATEGORY';
