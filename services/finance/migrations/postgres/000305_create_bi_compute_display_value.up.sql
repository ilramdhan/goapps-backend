-- Migration: create bi_compute_display_value function — applies sign convention per type/group.
BEGIN;

CREATE OR REPLACE FUNCTION bi_compute_display_value(
    p_type    VARCHAR,
    p_group_1 VARCHAR,
    p_group_2 VARCHAR,
    p_value   NUMERIC
) RETURNS NUMERIC AS $$
DECLARE
    v_display NUMERIC := p_value;
BEGIN
    -- MIS / EBITDA / NET PROFIT convention: flip INCOME to positive, cost stays negative for waterfall.
    IF p_type = 'MIS' AND p_group_1 IN ('EBITDA','NET PROFIT') THEN
        IF p_group_2 = 'INCOME' THEN
            v_display := -p_value;
        ELSIF p_group_2 LIKE '%COST%'
           OR p_group_2 LIKE '%CONSUMPTION%'
           OR p_group_2 IN ('MANPOWER','OVERHEADS','SELLING COST','BAD DEBT EXP') THEN
            v_display := -p_value;
        END IF;
    END IF;
    -- Default: return unchanged.
    RETURN v_display;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

COMMIT;
