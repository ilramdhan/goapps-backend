-- Seed canonical product types so /finance/product-master + /finance/product-orders
-- have lookups out of the box. Idempotent via ON CONFLICT on the unique code index.
INSERT INTO cost_product_type (cpt_type_code, cpt_type_name)
VALUES
    ('POY',    'Partially Oriented Yarn'),
    ('PTY',    'Polyester Textured Yarn'),
    ('TTY',    'Twisted Textured Yarn'),
    ('FG',     'Finished Goods'),
    ('INTER',  'Intermediate'),
    ('DTY',    'Drawn Textured Yarn'),
    ('FDY',    'Fully Drawn Yarn'),
    ('SDY',    'Spin Drawn Yarn'),
    ('ATY',    'Air Textured Yarn'),
    ('MEL',    'Melange'),
    ('TCM',    'Twisted Compound Multifilament'),
    ('TCY',    'Twisted Compound Yarn')
ON CONFLICT (cpt_type_code) DO NOTHING;
