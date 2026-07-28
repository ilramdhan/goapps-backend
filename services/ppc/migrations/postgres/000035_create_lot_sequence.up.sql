-- 000035: per-area, per-year lot number sequence.
--
-- Lot numbers were minted from time.Now().UnixNano(), producing a 22-digit
-- token no operator can transcribe onto a doff card. The Oracle ETL joins
-- production back to a work order by that lot string (MatchWO / MatchWOByLot),
-- and a mistyped lot surfaces only as a swallowed SYNC_FAILED row.
--
-- This table backs the transcribable form {AREA}{SEQ:04d}-{YY} (e.g. SPG0042-26),
-- max 12 chars, comfortably inside work_order.wo_lot_no VARCHAR(30).
--
-- The bump is INSERT ... ON CONFLICT DO UPDATE ... RETURNING, run inside the
-- work-order transaction, so the row lock serializes concurrent creates and a
-- rolled-back work order burns no lot number.

CREATE TABLE IF NOT EXISTS lot_sequence (
  ls_area_code CHAR(3)  NOT NULL,
  ls_year      SMALLINT NOT NULL,
  ls_last_seq  INT      NOT NULL DEFAULT 0,
  PRIMARY KEY (ls_area_code, ls_year)
);

COMMENT ON TABLE lot_sequence IS
  'Per-area, per-year counter backing transcribable PPC lot numbers ({AREA}{SEQ:04d}-{YY}). Bumped inside the work-order transaction.';
COMMENT ON COLUMN lot_sequence.ls_area_code IS 'PPC area: TXT, SPG or TWT.';
COMMENT ON COLUMN lot_sequence.ls_year IS 'Four-digit calendar year the sequence belongs to; the lot number carries its last two digits.';
COMMENT ON COLUMN lot_sequence.ls_last_seq IS 'Last sequence number handed out for this area+year.';
