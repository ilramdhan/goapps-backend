-- 000537 (backlog1 follow-up) — share the TTY and DTY TX Weight configs with
-- more product types (user decision 2026-09-30):
--   TTY config: TTY, TTS, TTM, TTH, TPY, TPS, TPM, TFY, TCY, TCS, TCM, TCH, PTS, ATT
--   DTY config: DTY, PTY, ATY, PLY, MEL
--   POY, ACY, ITY keep their own config (untouched here).
--
-- The target group is the live group that owns the anchor type (TTY / DTY),
-- resolved through the 000536 mapping — never by literal id. Types are joined
-- on cpt_type_code only.
--   * type code missing from cost_product_type  -> skipped with a NOTICE
--   * type already mapped to another group      -> skipped with a NOTICE
--     (ON CONFLICT (product_type_id) DO NOTHING — one config per type)
--   * anchor type without a group               -> whole list skipped, NOTICE
--
-- The grade weights of the newly mapped types change on purpose; recalculate
-- after deploy. Idempotent: an existing mapping is left as is.
BEGIN;

DO $$
DECLARE
    v_rec      RECORD;
    v_group    UUID;
    v_inserted INT;
    v_total    INT := 0;
BEGIN
    FOR v_rec IN
        SELECT v.anchor, v.type_code
        FROM (VALUES
            ('TTY', 'TTS'), ('TTY', 'TTM'), ('TTY', 'TTH'), ('TTY', 'TPY'), ('TTY', 'TPS'),
            ('TTY', 'TPM'), ('TTY', 'TFY'), ('TTY', 'TCY'), ('TTY', 'TCS'), ('TTY', 'TCM'),
            ('TTY', 'TCH'), ('TTY', 'PTS'), ('TTY', 'ATT'),
            ('DTY', 'PTY'), ('DTY', 'ATY'), ('DTY', 'PLY'), ('DTY', 'MEL')
        ) AS v(anchor, type_code)
    LOOP
        SELECT gt.ytwg_id INTO v_group
        FROM mst_yarn_tx_weight_group_type gt
        JOIN cost_product_type pt ON pt.cpt_type_id = gt.product_type_id
        JOIN mst_yarn_tx_weight_group g ON g.ytwg_id = gt.ytwg_id AND g.deleted_at IS NULL
        WHERE pt.cpt_type_code = v_rec.anchor;

        IF v_group IS NULL THEN
            RAISE NOTICE '000537: % has no TX Weight group — % not mapped.', v_rec.anchor, v_rec.type_code;
            CONTINUE;
        END IF;

        IF NOT EXISTS (SELECT 1 FROM cost_product_type WHERE cpt_type_code = v_rec.type_code) THEN
            RAISE NOTICE '000537: product type % not found in cost_product_type — skipped.', v_rec.type_code;
            CONTINUE;
        END IF;

        INSERT INTO mst_yarn_tx_weight_group_type (ytwg_id, product_type_id, created_by)
        SELECT v_group, pt.cpt_type_id, 'seed_000537'
        FROM cost_product_type pt
        WHERE pt.cpt_type_code = v_rec.type_code
        ON CONFLICT (product_type_id) DO NOTHING;
        GET DIAGNOSTICS v_inserted = ROW_COUNT;

        IF v_inserted = 0 AND NOT EXISTS (
            SELECT 1 FROM mst_yarn_tx_weight_group_type gt
            JOIN cost_product_type pt ON pt.cpt_type_id = gt.product_type_id
            WHERE pt.cpt_type_code = v_rec.type_code
              AND gt.ytwg_id = v_group
        ) THEN
            RAISE NOTICE '000537: product type % is already mapped to another TX Weight group — skipped.', v_rec.type_code;
        END IF;
        v_total := v_total + v_inserted;
    END LOOP;

    RAISE NOTICE '000537: mapped % additional product type(s) to the TTY/DTY TX Weight groups.', v_total;
END $$;

COMMIT;
