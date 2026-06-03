-- Revert 000358: restore period="ytd" on YTD EBITDA KPI.
BEGIN;

UPDATE bi_dashboard
SET kpi_config = jsonb_build_object('items', (
    SELECT jsonb_agg(
        CASE
            WHEN (kpi->>'label') = 'YTD EBITDA'
            THEN jsonb_set(kpi, '{period}', '"ytd"')
            ELSE kpi
        END
    )
    FROM jsonb_array_elements(kpi_config->'items') AS kpi
))
WHERE dashboard_code = 'EBITDA'
  AND kpi_config->'items' IS NOT NULL;

COMMIT;
