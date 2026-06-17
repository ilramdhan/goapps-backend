-- 000387: Product grade / quality-loss config. Feeds BC_PERC, NON_STD_PERC, BC_RECOVERY_RATE CAPP.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_product_grade (
    pg_id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    pg_code            VARCHAR(30)  NOT NULL,
    pg_name            VARCHAR(100) NOT NULL,
    pg_description     TEXT,
    bc_perc            NUMERIC(5,2) NOT NULL DEFAULT 0,
    non_std_perc       NUMERIC(5,2) NOT NULL DEFAULT 0,
    bc_recovery_rate   NUMERIC(5,2) NOT NULL DEFAULT 80,
    is_active          BOOLEAN      NOT NULL DEFAULT TRUE,
    notes              TEXT,
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by         VARCHAR(100) NOT NULL,
    updated_at         TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    deleted_at         TIMESTAMPTZ,
    deleted_by         VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_product_grade_code
    ON mst_product_grade (pg_code) WHERE deleted_at IS NULL;

INSERT INTO mst_product_grade (pg_code, pg_name, bc_perc, non_std_perc, bc_recovery_rate, created_by)
SELECT v.code, v.name, v.bc, v.ns, v.rec, 'seed_000387'
FROM (VALUES
    ('STD_A',   'Standard Grade A', 2.0, 3.0, 80.0),
    ('STD_B',   'Standard Grade B', 3.0, 5.0, 75.0),
    ('PREMIUM', 'Premium Grade',    1.0, 1.5, 90.0),
    ('MELANGE', 'Melange Grade',    2.5, 3.5, 78.0)
) AS v(code, name, bc, ns, rec)
WHERE NOT EXISTS (SELECT 1 FROM mst_product_grade p WHERE p.pg_code = v.code AND p.deleted_at IS NULL);

COMMIT;
