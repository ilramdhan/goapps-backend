-- 000366 down: remove the seeded test fill tasks and test product request.
DO $$
DECLARE
    v_request_id BIGINT;
BEGIN
    SELECT cpr_request_id
      INTO v_request_id
      FROM cost_product_request
     WHERE cpr_request_no = 'REQ-TEST-001';

    IF v_request_id IS NOT NULL THEN
        DELETE FROM cost_fill_task WHERE cft_request_id = v_request_id;
        DELETE FROM cost_product_request WHERE cpr_request_id = v_request_id;
    END IF;
END $$;
