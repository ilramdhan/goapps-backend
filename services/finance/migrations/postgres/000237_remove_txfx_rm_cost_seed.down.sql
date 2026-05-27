-- 000237 down: re-insert the TXFX_* invented rm cost rows so 000236 down (which
-- also deletes them via created_by) remains replayable independently. Same
-- shape + values as the original 000236 up insert block.

BEGIN;

INSERT INTO cst_rm_cost (
    period, rm_code, rm_type, item_code, rm_name, uom_code,
    cons_rate, cost_val, cost_mark, cost_sim,
    flag_valuation, flag_marketing, flag_simulation,
    flag_valuation_used, flag_marketing_used, flag_simulation_used,
    calculated_at, calculated_by, created_by
)
SELECT '202604', v.rm_code, 'ITEM', v.rm_code, v.rm_name, 'KG',
       v.rate, v.rate, v.rate, v.rate,
       'CONS','CONS','CONS','CONS','CONS','CONS',
       NOW(), 'seed_000236', 'seed_000236'
  FROM (VALUES
    ('TXFX_PTA',          'PTA Purified Terephthalic Acid',  12000),
    ('TXFX_MEG',          'MEG Mono Ethylene Glycol',        11000),
    ('TXFX_SPIN_OIL',     'Spin Finish Oil',                 25000),
    ('TXFX_TIO2',         'Titanium Dioxide (delustrant)',   42000),
    ('TXFX_DYE_BLK',      'Disperse Dyestuff Black',        180000),
    ('TXFX_DYE_NVY',      'Disperse Dyestuff Navy',         165000),
    ('TXFX_DYE_RED',      'Disperse Dyestuff Red',          195000),
    ('TXFX_DYE_WHT',      'Optical Brightener White',       240000),
    ('TXFX_DYE_GRY',      'Disperse Dyestuff Grey',         150000),
    ('TXFX_NAOH',         'Caustic Soda 50%',                 4500),
    ('TXFX_AC_ACID',      'Acetic Acid',                     12000),
    ('TXFX_SODA_ASH',     'Soda Ash',                         5000),
    ('TXFX_CONE',         'Paper Cone Tube',                  1500),
    ('TXFX_LEVELING',     'Leveling Agent',                  38000)
  ) AS v(rm_code, rm_name, rate)
ON CONFLICT (period, rm_code) DO NOTHING;

COMMIT;
