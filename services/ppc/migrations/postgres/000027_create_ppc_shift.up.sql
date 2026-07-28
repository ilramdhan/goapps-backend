-- PPC shift master. Genuine per-row master with real attributes (start/end time)
-- used by shift-entry and daily-performance. Codes 1/2/3 per PRD 09-master-data
-- shift config. ps_end_time may be earlier than ps_start_time to model a shift
-- that crosses midnight (e.g. shift 3: 22:00 -> 06:00 next day).
CREATE TABLE IF NOT EXISTS ppc_shift (
    ps_id         BIGSERIAL     PRIMARY KEY,
    ps_code       CHAR(1)       NOT NULL,
    ps_name       VARCHAR(40),
    ps_start_time TIME          NOT NULL,
    ps_end_time   TIME          NOT NULL,
    ps_is_active  BOOLEAN       NOT NULL DEFAULT TRUE,
    ps_created_at TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    ps_created_by VARCHAR(100)  NOT NULL DEFAULT 'system',
    ps_updated_at TIMESTAMPTZ,
    ps_updated_by VARCHAR(100),
    CONSTRAINT uq_ppc_shift_code UNIQUE (ps_code)
);
