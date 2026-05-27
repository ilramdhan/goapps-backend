-- The existing generate_cost_request_no() uses an atomic UPSERT counter table,
-- which is race-safe. The duplicate-key bug came from data drift: manual seeds
-- or restored backups inserted rows whose request_no suffix exceeded the
-- counter's last_number, so subsequent generated numbers collided.
--
-- This migration:
--   1. Re-syncs every (year_month, last_number) row to match the actual
--      MAX(suffix) across existing requests in that month.
--   2. Hardens the function so any future drift self-heals: GREATEST(counter, MAX).

BEGIN;

-- Re-sync existing counter rows.
INSERT INTO cost_request_no_counter (crnc_year_month, crnc_last_number)
SELECT
    SUBSTRING(cpr_request_no FROM 5 FOR 6) AS year_month,
    MAX(SUBSTRING(cpr_request_no FROM '\d+$')::INT) AS last_number
FROM cost_product_request
WHERE cpr_request_no LIKE 'REQ-______-____'
GROUP BY SUBSTRING(cpr_request_no FROM 5 FOR 6)
ON CONFLICT (crnc_year_month) DO UPDATE
SET crnc_last_number = GREATEST(cost_request_no_counter.crnc_last_number, EXCLUDED.crnc_last_number);

-- Harden the generator: take max(existing counter, existing max in table) + 1.
CREATE OR REPLACE FUNCTION generate_cost_request_no(p_clock TIMESTAMPTZ DEFAULT now())
RETURNS VARCHAR AS $$
DECLARE
    v_ym VARCHAR(6);
    v_n  INT;
    v_existing_max INT;
BEGIN
    v_ym := TO_CHAR(p_clock, 'YYYYMM');

    -- Defensive: look up the highest suffix that actually exists in the table
    -- for this month, so we never regenerate a colliding number even if the
    -- counter row drifted (e.g. after manual seed or partial restore).
    SELECT COALESCE(MAX(SUBSTRING(cpr_request_no FROM '\d+$')::INT), 0)
      INTO v_existing_max
      FROM cost_product_request
     WHERE cpr_request_no LIKE 'REQ-' || v_ym || '-%';

    INSERT INTO cost_request_no_counter (crnc_year_month, crnc_last_number)
    VALUES (v_ym, GREATEST(v_existing_max, 0) + 1)
    ON CONFLICT (crnc_year_month) DO UPDATE
    SET crnc_last_number = GREATEST(
        cost_request_no_counter.crnc_last_number,
        EXCLUDED.crnc_last_number - 1
    ) + 1
    RETURNING crnc_last_number INTO v_n;

    RETURN 'REQ-' || v_ym || '-' || LPAD(v_n::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION generate_cost_request_no IS
    'PRD Phase A — atomic REQ-YYYYMM-NNNN generator. Self-heals counter drift via MAX(existing suffix) lookup.';

COMMIT;
