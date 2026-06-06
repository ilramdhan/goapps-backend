-- 000365: Seed default global fill-assignment level configs for 3 route levels.
--
-- These rows act as the fallback fill config when no product-level or
-- request-level assignment override exists.  They can be edited at runtime
-- via the fill-assignment CRUD API.
--
-- Idempotency: guarded by NOT EXISTS on (clac_route_level) with clac_is_active=true,
-- matching the partial unique index uk_clac_level_active.
-- ON CONFLICT DO NOTHING would not fire for a partial-index conflict, so we use
-- an explicit guard.

DO $$
BEGIN
    -- Level 1: first-touch filler (e.g. Costing Dept analyst)
    IF NOT EXISTS (
        SELECT 1 FROM cost_level_assignment_config
        WHERE clac_route_level = 1 AND clac_is_active = true
    ) THEN
        INSERT INTO cost_level_assignment_config (
            clac_route_level,
            clac_filler_type,
            clac_filler_value,
            clac_approver_type,
            clac_approver_value,
            clac_reapprove_on_change,
            clac_sla_fill_hours,
            clac_sla_approve_hours,
            clac_is_active,
            clac_created_by,
            clac_updated_by
        ) VALUES (
            1,
            'DEPT',
            'COSTING',
            'DEPT',
            'COSTING_HEAD',
            false,
            48,
            24,
            true,
            'system',
            'system'
        );
    END IF;

    -- Level 2: second-touch filler (e.g. Engineering Dept)
    IF NOT EXISTS (
        SELECT 1 FROM cost_level_assignment_config
        WHERE clac_route_level = 2 AND clac_is_active = true
    ) THEN
        INSERT INTO cost_level_assignment_config (
            clac_route_level,
            clac_filler_type,
            clac_filler_value,
            clac_approver_type,
            clac_approver_value,
            clac_reapprove_on_change,
            clac_sla_fill_hours,
            clac_sla_approve_hours,
            clac_is_active,
            clac_created_by,
            clac_updated_by
        ) VALUES (
            2,
            'DEPT',
            'ENGINEERING',
            NULL,
            NULL,
            false,
            48,
            24,
            true,
            'system',
            'system'
        );
    END IF;

    -- Level 3: third-touch filler (e.g. Production Dept), longer SLA
    IF NOT EXISTS (
        SELECT 1 FROM cost_level_assignment_config
        WHERE clac_route_level = 3 AND clac_is_active = true
    ) THEN
        INSERT INTO cost_level_assignment_config (
            clac_route_level,
            clac_filler_type,
            clac_filler_value,
            clac_approver_type,
            clac_approver_value,
            clac_reapprove_on_change,
            clac_sla_fill_hours,
            clac_sla_approve_hours,
            clac_is_active,
            clac_created_by,
            clac_updated_by
        ) VALUES (
            3,
            'DEPT',
            'PRODUCTION',
            NULL,
            NULL,
            false,
            72,
            48,
            true,
            'system',
            'system'
        );
    END IF;
END $$;
