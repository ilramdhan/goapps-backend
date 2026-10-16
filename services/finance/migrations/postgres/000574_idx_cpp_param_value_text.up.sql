-- 000574: partial index on cost_product_parameter (cpp_param_id, cpp_value_text)
-- supporting the master-lookup-sync "which products reference master X" lookup.
--
-- WHY: after a lookup master (machine, intermingling, product grade, MB head,
-- MB spin, box/bobbin cost) is saved, the sync usage-count / plan finds trigger
-- cells with `cpp_param_id IN (trigger params) AND cpp_value_text = <master key>`.
-- Existing indexes cover only cpp_param_id (idx_cpp_param, 000217) and
-- cpp_value_mb_spin_id (idx_cpp_value_mb_spin_id, 000494); neither narrows by
-- the stored key text, so the lookup scanned every row of the param.
--
-- Partial (text IS NOT NULL): numeric/flag rows never match a key lookup.
-- Plain CREATE INDEX (no CONCURRENTLY): golang-migrate runs inside a tx.
-- lock_timeout bounds the wait for the SHARE lock so a busy table fails fast
-- instead of stalling writers behind a queued index build.
BEGIN;
SET LOCAL lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_cpp_param_value_text
    ON cost_product_parameter (cpp_param_id, cpp_value_text)
    WHERE cpp_value_text IS NOT NULL;

COMMIT;
