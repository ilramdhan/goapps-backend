-- Canonical PRD Phase B §7.2.3 initial seed — 4 default RM types.
-- Admin can add more later via UI. Seeds are idempotent (ON CONFLICT DO NOTHING).

INSERT INTO cost_rm_type (crmt_type_code, crmt_type_name, crmt_reference_target, crmt_allow_sub_sequence)
VALUES
    ('STORE_RATE',   'Store Rate',   'MASTER',  FALSE),
    ('CAPTIVE_COST', 'Captive Cost', 'PRODUCT', FALSE),
    ('MULTI_YARN',   'Multi Yarn',   'PRODUCT', TRUE),
    ('UNEVEN_PACK',  'Uneven Packing','PRODUCT', FALSE)
ON CONFLICT (crmt_type_code) DO NOTHING;
