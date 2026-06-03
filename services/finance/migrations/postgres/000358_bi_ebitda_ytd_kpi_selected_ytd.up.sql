-- Migration 000358: Switch EBITDA "YTD EBITDA" KPI from period="ytd" to "selected_ytd".
--
-- "ytd" always uses today's date as the anchor (Jan 1 2026 → today).
-- "selected_ytd" uses the viewer's selected month as the anchor:
--   May 2026 selected → Jan 1 2026 → May 31 2026  (compare via YTD_vs_LY → Jan-May 2025)
--   May 2025 selected → Jan 1 2025 → May 31 2025  (compare → Jan-May 2024)
--   no month selected → Jan 1 current_year → today (same as "ytd")
--
-- The compare mode stays "YTD_vs_LY" (shifts the YTD period back 1 year).

BEGIN;

UPDATE bi_dashboard
SET kpi_config = (
    SELECT jsonb_agg(
        CASE
            WHEN (kpi->>'label') = 'YTD EBITDA'
            THEN jsonb_set(kpi, '{period}', '"selected_ytd"')
            ELSE kpi
        END
    )
    FROM jsonb_array_elements(kpi_config->'items') AS kpi
)
WHERE dashboard_code = 'EBITDA'
  AND kpi_config->'items' IS NOT NULL;

COMMIT;
