-- 000413: Add CMBS_ORION_ITEM_CODE to mst_mb_spin for product params lookup.
-- Oracle product params reference MB_SPIN via CMBS_ORION_ITEM_CODE (ERP item code),
-- not via CMBS_MB_COSTING. This column enables GetByOrionItemCode lookup.

ALTER TABLE public.mst_mb_spin
  ADD COLUMN IF NOT EXISTS mbs_orion_item_code VARCHAR(200);

CREATE INDEX IF NOT EXISTS idx_mst_mb_spin_orion_item_code
  ON public.mst_mb_spin(mbs_orion_item_code)
  WHERE (mbs_orion_item_code IS NOT NULL AND deleted_at IS NULL);

COMMENT ON COLUMN public.mst_mb_spin.mbs_orion_item_code
  IS 'Oracle CMBS_ORION_ITEM_CODE — ERP item code used as product-params lookup key';
