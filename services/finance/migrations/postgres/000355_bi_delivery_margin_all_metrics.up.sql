-- Migration 000355: Delivery Margin chart — 4 core USD metrics per UX design.
--
-- Based on delivery_margin_final.html design: show Gross Sales, Net Sales,
-- Production Cost, and Margin as four trend lines.
--
-- Rationale:
--   GROSS_SALES  — total revenue before selling cost deduction
--   NETT_SALES   — revenue after selling cost (GROSS_SALES - SELLING_COST)
--   COST_PROD    — production + delivery costs (negative values)
--   MARGIN       — NETT_SALES - COST_PROD (the actual delivery margin)
--
-- Excluded intentionally:
--   SELLING_COST — already implicit in the GROSS_SALES vs NETT_SALES gap
--   QUANTITY     — KGS unit; cannot share the same currency value axis

BEGIN;

UPDATE bi_dashboard
SET chart_config = jsonb_set(
    chart_config,
    '{metric_filter,include_metrics}',
    '["GROSS_SALES", "NETT_SALES", "COST_PROD", "MARGIN"]'::jsonb
)
WHERE dashboard_code = 'DELIVERY_MARGIN';

COMMIT;
