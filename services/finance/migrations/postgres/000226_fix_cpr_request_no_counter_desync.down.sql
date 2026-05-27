-- Restore the previous (non-self-healing) generator. Counter sync rows stay
-- as they are; the function is reverted only.

CREATE OR REPLACE FUNCTION generate_cost_request_no(p_clock TIMESTAMPTZ DEFAULT now())
RETURNS VARCHAR AS $$
DECLARE
    v_ym VARCHAR(6);
    v_n  INT;
BEGIN
    v_ym := TO_CHAR(p_clock, 'YYYYMM');
    INSERT INTO cost_request_no_counter (crnc_year_month, crnc_last_number)
    VALUES (v_ym, 1)
    ON CONFLICT (crnc_year_month)
    DO UPDATE SET crnc_last_number = cost_request_no_counter.crnc_last_number + 1
    RETURNING crnc_last_number INTO v_n;
    RETURN 'REQ-' || v_ym || '-' || LPAD(v_n::TEXT, 4, '0');
END;
$$ LANGUAGE plpgsql;
