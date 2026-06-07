-- 000366: Seed test fill-assignment tasks for CRUD + lifecycle testing.
--
-- Creates ONE test cost_product_request (REQ-TEST-001) and THREE cost_fill_task
-- rows (one per route level), each in a different status to allow testing the
-- full state machine:
--   Level 1 -> ACTIVE   (just created, not yet claimed)
--   Level 2 -> FILLING  (claimed, in progress)
--   Level 3 -> FILLED   (all params filled, awaiting approval)
--
-- Pre-condition: requires at least ONE cost_product_master + cost_route_head row.
-- Uses the first available COMPLETE route head.  If none exists, the block is
-- a no-op (idempotent skip).
--
-- Idempotency: guarded by NOT EXISTS on uk_cft_request_level and cpr_request_no.

DO $$
DECLARE
    v_request_id   BIGINT;
    v_request_no   VARCHAR(30) := 'REQ-TEST-001';
    v_route_id     BIGINT;
    v_type_id      INT;
    v_product_id   BIGINT;
BEGIN
    -- -------------------------------------------------------------------------
    -- 0. Resolve dependencies — skip entire block if prerequisites are missing.
    -- -------------------------------------------------------------------------

    -- Pick the first COMPLETE route head (seeded by 000236 / 000239 / 000244)
    SELECT crh_head_id, crh_product_sys_id
      INTO v_route_id, v_product_id
      FROM cost_route_head
     WHERE crh_routing_status = 'COMPLETE'
       AND crh_deleted_at IS NULL
     ORDER BY crh_head_id
     LIMIT 1;

    IF v_route_id IS NULL THEN
        RAISE NOTICE '000366 seed skipped: no COMPLETE cost_route_head found.';
        RETURN;
    END IF;

    -- Resolve the QUOTE request type (always seeded by 000200)
    SELECT crt_type_id
      INTO v_type_id
      FROM cost_request_type
     WHERE crt_code = 'QUOTE'
     LIMIT 1;

    IF v_type_id IS NULL THEN
        RAISE NOTICE '000366 seed skipped: cost_request_type QUOTE not found.';
        RETURN;
    END IF;

    -- -------------------------------------------------------------------------
    -- 1. Create the test product request (skip if already exists).
    -- -------------------------------------------------------------------------
    IF NOT EXISTS (
        SELECT 1 FROM cost_product_request WHERE cpr_request_no = v_request_no
    ) THEN
        INSERT INTO cost_product_request (
            cpr_request_no,
            cpr_request_type_id,
            cpr_title,
            cpr_description,
            cpr_customer_name,
            cpr_customer_code,
            cpr_product_classification,
            cpr_urgency_level,
            cpr_status,
            cpr_requester_user_id
        ) VALUES (
            v_request_no,
            v_type_id,
            '[TEST] Fill-assignment lifecycle test request',
            'Seeded by 000366 for testing fill-task CRUD and status transitions.',
            'Test Customer',
            'TEST-CUST',
            'existing',
            'medium',
            'ROUTING_DEFINED',
            'system'
        )
        RETURNING cpr_request_id INTO v_request_id;
    ELSE
        SELECT cpr_request_id
          INTO v_request_id
          FROM cost_product_request
         WHERE cpr_request_no = v_request_no;
    END IF;

    -- -------------------------------------------------------------------------
    -- 2. Create fill tasks for levels 1-3.
    -- -------------------------------------------------------------------------

    -- Level 1 — ACTIVE (freshly assigned, waiting for a filler to claim it)
    IF NOT EXISTS (
        SELECT 1 FROM cost_fill_task
         WHERE cft_request_id = v_request_id AND cft_route_level = 1
    ) THEN
        INSERT INTO cost_fill_task (
            cft_request_id,
            cft_route_head_id,
            cft_route_level,
            cft_filler_type,
            cft_filler_value,
            cft_approver_type,
            cft_approver_value,
            cft_reapprove_on_change,
            cft_sla_fill_hours,
            cft_sla_approve_hours,
            cft_status,
            cft_total_params,
            cft_filled_params
        ) VALUES (
            v_request_id,
            v_route_id,
            1,
            'DEPT',
            'COSTING',
            'DEPT',
            'COSTING_HEAD',
            false,
            48,
            24,
            'ACTIVE',
            5,
            0
        );
    END IF;

    -- Level 2 — FILLING (claimed by a filler, partially filled)
    IF NOT EXISTS (
        SELECT 1 FROM cost_fill_task
         WHERE cft_request_id = v_request_id AND cft_route_level = 2
    ) THEN
        INSERT INTO cost_fill_task (
            cft_request_id,
            cft_route_head_id,
            cft_route_level,
            cft_filler_type,
            cft_filler_value,
            cft_approver_type,
            cft_approver_value,
            cft_reapprove_on_change,
            cft_sla_fill_hours,
            cft_sla_approve_hours,
            cft_status,
            cft_total_params,
            cft_filled_params,
            cft_claimed_by,
            cft_claimed_at
        ) VALUES (
            v_request_id,
            v_route_id,
            2,
            'DEPT',
            'ENGINEERING',
            NULL,
            NULL,
            false,
            48,
            24,
            'FILLING',
            8,
            3,
            'system',
            NOW() - INTERVAL '4 hours'
        );
    END IF;

    -- Level 3 — FILLED (all params filled, approval pending)
    IF NOT EXISTS (
        SELECT 1 FROM cost_fill_task
         WHERE cft_request_id = v_request_id AND cft_route_level = 3
    ) THEN
        INSERT INTO cost_fill_task (
            cft_request_id,
            cft_route_head_id,
            cft_route_level,
            cft_filler_type,
            cft_filler_value,
            cft_approver_type,
            cft_approver_value,
            cft_reapprove_on_change,
            cft_sla_fill_hours,
            cft_sla_approve_hours,
            cft_status,
            cft_total_params,
            cft_filled_params,
            cft_claimed_by,
            cft_claimed_at,
            cft_filled_at
        ) VALUES (
            v_request_id,
            v_route_id,
            3,
            'DEPT',
            'PRODUCTION',
            NULL,
            NULL,
            false,
            72,
            48,
            'FILLED',
            6,
            6,
            'system',
            NOW() - INTERVAL '26 hours',
            NOW() - INTERVAL '2 hours'
        );
    END IF;

    RAISE NOTICE '000366 seed complete: request_id=%, route_head_id=%', v_request_id, v_route_id;
END $$;
