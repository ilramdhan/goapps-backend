-- Lot master. PPC-owned. Lot key = item_code + shade_code. Standard weights drive
-- TXT qty calculation (full/unfull bobbins × std weight).
CREATE TABLE IF NOT EXISTS lot_master (
    lm_lot_no            VARCHAR(30)  PRIMARY KEY,
    lm_item_code         VARCHAR(30)  NOT NULL,
    lm_shade_code        VARCHAR(20)  NOT NULL,
    lm_std_weight_full   DECIMAL(8,4) NOT NULL,   -- TXT: TRN_STS=0 (Full bobbin)
    lm_std_weight_unfull DECIMAL(8,4) NOT NULL,   -- TXT: TRN_STS=1 (Unfull bobbin)
    lm_notes             TEXT,
    lm_created_by        VARCHAR(100) NOT NULL DEFAULT 'system',
    lm_created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    lm_updated_at        TIMESTAMPTZ,
    lm_updated_by        VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_lot_master_item_shade ON lot_master (lm_item_code, lm_shade_code);
