-- 000527 — indexes supporting the Cost Results list filters
-- (cost_result_repository.go ListResults / cpcListWhere).
--
-- idx_cpc_rm_cost_detail_gin: GIN(jsonb_path_ops) index on
-- cst_product_cost.cpc_rm_cost_detail. This makes jsonb containment (`@>`)
-- predicates against the column indexable — see
-- rmGroupCodeContainsClause in cost_result_repository.go, which now emits
-- `cpc.cpc_rm_cost_detail @> '[{"rm_type": "GROUP", "ref_code": "..."}]'::jsonb`
-- for the RM-group multi-select filter instead of unnesting the array with
-- jsonb_array_elements(...) = ANY(...) (which cannot use any index).
--
-- This index does NOT help the free-text `search` param's RM matching
-- (rmDetailExistsClause): that clause matches with ILIKE against a
-- per-element resolved display name (RM group/item/product name), which is
-- pattern matching over a joined value, not a containment check against the
-- JSONB document itself — no jsonb index (GIN or otherwise) accelerates
-- that shape. That path stays a per-row jsonb_array_elements scan, same as
-- before; it is deliberately not addressed here, and stays scoped by
-- period/status like every other row in the list.
--
-- idx_cost_product_master_shade_code: plain btree on cpm_shade_code,
-- supporting the shade multi-select filter's `= ANY($n)` equality predicate.
--
-- Note for whoever applies this migration: cst_product_cost /
-- cost_product_master are live, actively-written tables in production. If
-- their row counts are large enough that a plain CREATE INDEX's ACCESS
-- EXCLUSIVE lock would cause a noticeable write stall, prefer running the
-- equivalent CREATE INDEX CONCURRENTLY manually outside of a transaction
-- block (golang-migrate runs each file in a transaction, which does not
-- support CONCURRENTLY) rather than applying this file as-is during
-- business hours.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_cpc_rm_cost_detail_gin
    ON cst_product_cost USING GIN (cpc_rm_cost_detail jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_cost_product_master_shade_code
    ON cost_product_master (cpm_shade_code)
    WHERE cpm_shade_code IS NOT NULL;

COMMIT;
