-- Reverse of 000081: remove the PPC Customer master menu, the customer CRUD
-- permissions, and the `.sync` permissions. Also restores chk_permission_action
-- to its exact pre-000081 state (the 35-value set left by 000068 — without 'sync').

-- 1. role_permissions for the permissions introduced by 000081
DELETE FROM role_permissions WHERE permission_id IN (
  SELECT permission_id FROM mst_permission WHERE permission_code IN (
    'ppc.master.customer.view','ppc.master.customer.create',
    'ppc.master.customer.update','ppc.master.customer.delete',
    'ppc.master.machine.sync','ppc.master.customer.sync','ppc.master.lot.sync'
  )
);

-- 2. menu_permissions for the new menu
DELETE FROM menu_permissions WHERE menu_id IN (
  SELECT menu_id FROM mst_menu WHERE menu_code = 'PPC_MASTER_CUSTOMER'
);

-- 3. menu
DELETE FROM mst_menu WHERE menu_code = 'PPC_MASTER_CUSTOMER';

-- 4. permissions
DELETE FROM mst_permission WHERE permission_code IN (
  'ppc.master.customer.view','ppc.master.customer.create',
  'ppc.master.customer.update','ppc.master.customer.delete',
  'ppc.master.machine.sync','ppc.master.customer.sync','ppc.master.lot.sync'
);

-- 5. RESTORE chk_permission_action — remove 'sync' (back to the 000068 set)
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute'
  ));
