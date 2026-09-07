-- Migration: Data-only backfill of cst_rm_group_head_period / cst_rm_group_detail_period
-- from existing cst_rm_cost / cst_rm_cost_detail history, per
-- docs/superpowers/specs/2026-09-05-rm-group-period-versioning-design.md §3.
--
-- For every (period, group_head_id) that was ever calculated (present in
-- cst_rm_cost), insert a snapshot row using the V2 numeric inputs actually
-- used to calculate that period (from cst_rm_cost / cst_rm_cost_detail),
-- falling back to the anchor row (cst_rm_group_head / cst_rm_group_detail)
-- for V1 fields that were never period-snapshotted anywhere. Every row this
-- migration inserts is marked is_backfilled = true, since even the
-- cst_rm_cost-sourced columns are a "best available reconstruction," not a
-- row created by an actual user edit under this mechanism. Idempotent via
-- ON CONFLICT DO NOTHING — safe to re-run.
--
-- Periods with no cst_rm_cost history at all (group created but never
-- calculated) are intentionally NOT backfilled — per the read fallback rule,
-- such periods correctly resolve to "inherits current config" with no
-- period-row present.

-- 1) cst_rm_group_head_period — one row per (period, group_head_id) sourced
--    from the GROUP-type cst_rm_cost row for that period (rm_code = group_code
--    in phase 1, so uk_rm_cost_period_rm already guarantees at most one such
--    row per (period, group_head_id)).
INSERT INTO cst_rm_group_head_period (
    group_head_period_id,
    period,
    group_head_id,
    name,
    description,
    colorant,
    ci_name,
    cost_percentage,
    cost_per_kg,
    flag_valuation,
    flag_marketing,
    flag_simulation,
    init_val_valuation,
    init_val_marketing,
    init_val_simulation,
    is_active,
    marketing_freight_rate,
    marketing_anti_dumping_pct,
    marketing_default_value,
    valuation_flag_v2,
    marketing_flag_v2,
    is_backfilled,
    created_at,
    created_by
)
SELECT
    gen_random_uuid(),
    c.period,
    c.group_head_id,
    h.group_name,
    h.description,
    h.colourant,
    h.ci_name,
    h.cost_percentage,
    h.cost_per_kg,
    h.flag_valuation,
    h.flag_marketing,
    h.flag_simulation,
    h.init_val_valuation,
    h.init_val_marketing,
    h.init_val_simulation,
    h.is_active,
    COALESCE(c.marketing_freight_rate, h.marketing_freight_rate),
    COALESCE(c.marketing_anti_dumping_pct, h.marketing_anti_dumping_pct),
    COALESCE(c.marketing_default_value, h.marketing_default_value),
    COALESCE(c.valuation_flag_v2, h.valuation_flag),
    COALESCE(c.marketing_flag_v2, h.marketing_flag),
    true,
    NOW(),
    'system'
FROM cst_rm_cost c
JOIN cst_rm_group_head h ON h.group_head_id = c.group_head_id
WHERE c.rm_type = 'GROUP'
  AND c.group_head_id IS NOT NULL
ON CONFLICT (period, group_head_id) DO NOTHING;

-- 2) cst_rm_group_detail_period — one row per (period, group_detail_id) sourced
--    from cst_rm_cost_detail's per-item snapshot. A single item/period can have
--    more than one cst_rm_cost_detail row (multi-grade blending via grade_code),
--    so rank and pick one deterministically (most recently created) per
--    (period, group_detail_id) before inserting, to avoid violating the
--    (period, group_detail_id) unique index.
WITH ranked_detail AS (
    SELECT
        cd.period,
        cd.group_detail_id,
        cd.freight_rate,
        cd.anti_dumping_pct,
        cd.duty_pct,
        cd.transport_rate,
        cd.valuation_default_value,
        ROW_NUMBER() OVER (
            PARTITION BY cd.period, cd.group_detail_id
            ORDER BY cd.created_at DESC, cd.cost_detail_id DESC
        ) AS rn
    FROM cst_rm_cost_detail cd
    WHERE cd.group_detail_id IS NOT NULL
)
INSERT INTO cst_rm_group_detail_period (
    group_detail_period_id,
    period,
    group_detail_id,
    market_percentage,
    market_value_rp,
    sort_order,
    is_active,
    is_dummy,
    valuation_freight_rate,
    valuation_anti_dumping_pct,
    valuation_duty_pct,
    valuation_transport_rate,
    valuation_default_value,
    is_backfilled,
    created_at,
    created_by
)
SELECT
    gen_random_uuid(),
    rd.period,
    rd.group_detail_id,
    d.market_percentage,
    d.market_value_rp,
    d.sort_order,
    d.is_active,
    d.is_dummy,
    rd.freight_rate,
    rd.anti_dumping_pct,
    rd.duty_pct,
    rd.transport_rate,
    rd.valuation_default_value,
    true,
    NOW(),
    'system'
FROM ranked_detail rd
JOIN cst_rm_group_detail d ON d.group_detail_id = rd.group_detail_id
WHERE rd.rn = 1
ON CONFLICT (period, group_detail_id) DO NOTHING;
