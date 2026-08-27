-- K-31 (§11 butir 115): index the mst_mb_head.mbh_cost_product_id link column.
--
-- Column: mst_mb_head.mbh_cost_product_id BIGINT, added in 000445_extend_mst_mb_head_workflow
-- without any index. It is the soft link from an MB Head to its auto-generated
-- cost_product_master.cpm_product_sys_id, written back once by mbWriteBackCostProduct
-- (internal/infrastructure/postgres/mb_autogen_repository.go) at the DRAFT->VALIDATED
-- transition, and NULL for every head that has not been validated yet.
--
-- Why PARTIAL (WHERE ... IS NOT NULL): the column is nullable and, by the workflow above,
-- the majority of heads carry NULL until validation. This mirrors the existing convention on
-- this same table — idx_mbh_lusture_code in 000445 is written exactly this way. A partial
-- index keeps only the linked rows, and the queries below never look for NULL.
--
-- Queries that benefit:
--   * internal/infrastructure/postgres/mb_autogen_repository.go:224 — mbResolveRefProductSysID,
--     the nested-MB reference resolution (reads the link per referenced head).
--   * internal/infrastructure/postgres/cst_mb_cost_repository.go:61 — ListStalePushedMBHIDs,
--     correlated EXISTS on cst_product_cost.cpc_product_sys_id = mbh.mbh_cost_product_id.
--   * internal/infrastructure/postgres/mb_recipe_full_export_repository.go:96 — full recipe
--     export, LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.mbh_cost_product_id.
--   * The reverse lookup described in §11 butir 116 ("from a cost product's detail page, link
--     back to its MB Head") — i.e. WHERE mbh_cost_product_id = <sys_id>. No such query exists
--     in internal/ today; this index is what makes it a plain index seek when it is added.
--
-- NON-UNIQUE on purpose. The Go write path does look 1:1 (TransitionWithAutoGen skips auto-gen
-- when entity.CostProductID() != 0, and mbInsertCostProductMaster INSERTs a fresh
-- cost_product_master row per head), but there is no existing DB constraint asserting that and
-- no way to confirm it for already-migrated/backfilled production rows from the code alone. A
-- UNIQUE index that is wrong would abort this migration against real data, so uniqueness is
-- deliberately NOT asserted here.
--
-- Plain CREATE INDEX, not CONCURRENTLY. Evidence, not assumption:
--   * The runner is the golang-migrate CLI v4.18.1 (Makefile MIGRATE_VERSION / migrate-up).
--   * Its postgres driver (database/postgres/postgres.go, Run/runStatement) sends the whole
--     .sql file to the server as ONE Exec unless x-multi-statement is set — and this repo's
--     migrate-up DSN does not set x-multi-statement. A multi-statement simple-query Exec is
--     executed by PostgreSQL as an implicit transaction block, and CREATE INDEX CONCURRENTLY
--     is rejected inside a transaction block.
--   * No migration in this directory uses CREATE INDEX CONCURRENTLY. The only CONCURRENTLY
--     uses (000310, 000322, 000354) are REFRESH MATERIALIZED VIEW inside a function body.
-- With no precedent, this migration does not become the first. Trade-off to be aware of: a
-- plain CREATE INDEX takes a brief ACCESS EXCLUSIVE lock on mst_mb_head, blocking writes to
-- the table while the index builds. Run it in a low-traffic window.
-- BEGIN/COMMIT is explicit to match the nearest sibling, 000482_index_mbh_vs_number, the
-- other index-only migration on this same table.
BEGIN;

CREATE INDEX IF NOT EXISTS idx_mbh_cost_product_id
    ON mst_mb_head (mbh_cost_product_id)
    WHERE mbh_cost_product_id IS NOT NULL;

COMMIT;
