-- Small master used by cost_product_spec (PRD Phase A §7.1.2 CPS_paper_tube_type_id).
-- Note: the canonical PRD references a generic "master_paper_tube"; we ship as cost_paper_tube_type
-- under the finance schema since this lives in the finance service.
CREATE TABLE IF NOT EXISTS cost_paper_tube_type (
    cptt_paper_tube_type_id SERIAL       PRIMARY KEY,
    cptt_code               VARCHAR(30)  NOT NULL,
    cptt_display_name       VARCHAR(100) NOT NULL,
    cptt_is_active          BOOLEAN      NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_paper_tube_type_code ON cost_paper_tube_type (cptt_code);

INSERT INTO cost_paper_tube_type (cptt_code, cptt_display_name)
VALUES
    ('JUMBO_3IN',  '3 inch jumbo'),
    ('JUMBO_4IN',  '4 inch jumbo'),
    ('NORMAL_3IN', '3 inch normal'),
    ('NORMAL_4IN', '4 inch normal'),
    ('PALLET',     'Pallet (no tube)')
ON CONFLICT (cptt_code) DO NOTHING;

COMMENT ON TABLE cost_paper_tube_type IS 'PRD Phase A — paper tube master used by cost_product_spec.';
