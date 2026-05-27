-- Document the semantics of the stock_* columns in cst_rm_cost_detail.
-- The V2 RM cost engine merges Oracle's STORES + DEPT into a single "stock"
-- bucket per item before persisting these snapshot rows, because in the
-- business model stock is one physical inventory; STORES vs DEPT is a
-- reporting split only. The source table cst_item_cons_stk_po keeps the
-- split untouched for grouping/audit reports.

COMMENT ON COLUMN cst_rm_cost_detail.stock_val IS
    'Stock total per item — merged stores_val + dept_val from cst_item_cons_stk_po at calc time. Source split is preserved upstream for audit/grouping reports.';

COMMENT ON COLUMN cst_rm_cost_detail.stock_qty IS
    'Stock total per item — merged stores_qty + dept_qty from cst_item_cons_stk_po at calc time. Source split is preserved upstream.';

COMMENT ON COLUMN cst_rm_cost_detail.stock_rate IS
    'Per-item stock rate = stock_val / stock_qty (both already merged stores+dept). Group-total SR is Σstock_val / Σstock_qty across items.';
