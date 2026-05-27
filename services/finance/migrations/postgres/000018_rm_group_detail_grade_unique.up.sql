-- Migration: evolve the "one item per active group" rule to include grade_code.
--
-- Context: the Oracle sync feed (cst_item_cons_stk_po) keys items on
-- (item_code, grade_code) — the SAME item_code can have multiple grade
-- variants with different qty/val/rate snapshots. The original grouping rule
-- collapsed all variants into one row, which caused:
--   1) Picker displayed four rows, user picked the "enriched" variant, save
--      used the wrong variant's metadata (arbitrary first-row).
--   2) After grouping one variant, the other variants disappeared from the
--      "Ungrouped Items" report.
--
-- Fix: treat (item_code, grade_code) as the business natural key for
-- grouping. A NULL/empty grade_code behaves as its own variant. The partial
-- unique index is rebuilt accordingly.

DROP INDEX IF EXISTS uk_rm_group_detail_item_active;

-- COALESCE pins a NULL grade_code to the empty string so a single
-- no-grade variant still has exactly one active row. Expression indexes
-- require IMMUTABLE functions, and COALESCE with a literal qualifies.
CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_group_detail_item_grade_active
    ON cst_rm_group_detail (item_code, COALESCE(grade_code, ''))
    WHERE deleted_at IS NULL AND is_active = true;

-- Replace the item-only lookup index with a composite (item_code, grade_code)
-- index to serve the new natural-key queries efficiently.
DROP INDEX IF EXISTS idx_rm_group_detail_item;
CREATE INDEX IF NOT EXISTS idx_rm_group_detail_item_grade
    ON cst_rm_group_detail (item_code, grade_code) WHERE deleted_at IS NULL;
