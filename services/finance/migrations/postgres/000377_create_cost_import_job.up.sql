CREATE TABLE IF NOT EXISTS cost_import_job (
    cij_job_id        BIGSERIAL    PRIMARY KEY,
    cij_entity        VARCHAR(30)  NOT NULL,
    cij_status        VARCHAR(20)  NOT NULL DEFAULT 'PENDING',
    cij_total_rows    INT          NOT NULL DEFAULT 0,
    cij_processed     INT          NOT NULL DEFAULT 0,
    cij_success       INT          NOT NULL DEFAULT 0,
    cij_failed        INT          NOT NULL DEFAULT 0,
    cij_skipped       INT          NOT NULL DEFAULT 0,
    cij_file_key      TEXT,
    cij_error_file    TEXT,
    cij_error_detail  TEXT,
    cij_created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    cij_created_by    VARCHAR(100) NOT NULL,
    cij_started_at    TIMESTAMPTZ,
    cij_completed_at  TIMESTAMPTZ,
    cij_parent_job_id BIGINT REFERENCES cost_import_job(cij_job_id),
    CONSTRAINT chk_cij_status CHECK (cij_status IN ('PENDING','RUNNING','DONE','FAILED','PARTIAL')),
    CONSTRAINT chk_cij_entity CHECK (cij_entity IN ('product_type','parameter','product_master','capp','cpp'))
);

CREATE INDEX IF NOT EXISTS idx_cost_import_job_status   ON cost_import_job (cij_status);
CREATE INDEX IF NOT EXISTS idx_cost_import_job_entity   ON cost_import_job (cij_entity);
CREATE INDEX IF NOT EXISTS idx_cost_import_job_created  ON cost_import_job (cij_created_at DESC);

COMMENT ON TABLE  cost_import_job               IS 'Tracks async import job lifecycle for costing master data.';
COMMENT ON COLUMN cost_import_job.cij_entity    IS 'product_type | parameter | product_master | capp | cpp';
COMMENT ON COLUMN cost_import_job.cij_status    IS 'PENDING | RUNNING | DONE | FAILED | PARTIAL';
COMMENT ON COLUMN cost_import_job.cij_file_key  IS 'MinIO object key of the uploaded Excel file.';
COMMENT ON COLUMN cost_import_job.cij_error_file IS 'MinIO object key of the generated error-report Excel.';
