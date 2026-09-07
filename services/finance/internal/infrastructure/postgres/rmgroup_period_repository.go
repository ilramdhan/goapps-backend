// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// =============================================================================
// Period-scoped snapshot operations
// =============================================================================
//
// Implements the 5 Repository methods added for rm-group-period-versioning
// (see docs/superpowers/specs/2026-09-05-rm-group-period-versioning-design.md
// §6). Kept in its own file — per goapps-backend/CLAUDE.md's 400-line
// repository guideline — so rmgroup_head_repository.go / rmgroup_detail_repository.go
// stay at their current size. Uses the same *RMGroupRepository receiver as the
// head/detail methods; the compile-time interface assertion
// (`var _ rmgroup.Repository = (*RMGroupRepository)(nil)`) already lives in
// rmgroup_detail_repository.go.

const headPeriodSelectSQL = `
	SELECT group_head_period_id, period, group_head_id,
	       name, description, colorant, ci_name,
	       cost_percentage, cost_per_kg,
	       flag_valuation, flag_marketing, flag_simulation,
	       init_val_valuation, init_val_marketing, init_val_simulation,
	       marketing_freight_rate, marketing_anti_dumping_pct, marketing_default_value,
	       valuation_flag_v2, marketing_flag_v2,
	       is_backfilled, created_at, created_by, updated_at, updated_by
	FROM cst_rm_group_head_period`

// GetHeadPeriodSnapshot returns the period-scoped snapshot for (headID, period).
// Returns rmgroup.ErrNotFound when no snapshot exists yet for that period —
// callers fall back to rmgroup.NewHeadPeriodSnapshotFromHead.
func (r *RMGroupRepository) GetHeadPeriodSnapshot(ctx context.Context, headID uuid.UUID, period string) (*rmgroup.HeadPeriodSnapshot, error) {
	row := r.db.QueryRowContext(ctx,
		headPeriodSelectSQL+` WHERE group_head_id = $1 AND period = $2`,
		headID, period,
	)
	return scanHeadPeriodSnapshot(row)
}

// UpsertHeadPeriod writes the snapshot keyed on (period, group_head_id).
// created_at/created_by are preserved on conflict (first-write wins); every
// other editable column is overwritten — mirrors rmcost_repository.go's
// upsertCost ON CONFLICT idiom.
func (r *RMGroupRepository) UpsertHeadPeriod(ctx context.Context, snap *rmgroup.HeadPeriodSnapshot) error {
	mi := snap.MarketingInputs
	query := `
		INSERT INTO cst_rm_group_head_period (
			group_head_period_id, period, group_head_id,
			name, description, colorant, ci_name,
			cost_percentage, cost_per_kg,
			flag_valuation, flag_marketing, flag_simulation,
			init_val_valuation, init_val_marketing, init_val_simulation,
			marketing_freight_rate, marketing_anti_dumping_pct, marketing_default_value,
			valuation_flag_v2, marketing_flag_v2,
			is_backfilled, created_at, created_by, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		ON CONFLICT (period, group_head_id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			colorant = EXCLUDED.colorant,
			ci_name = EXCLUDED.ci_name,
			cost_percentage = EXCLUDED.cost_percentage,
			cost_per_kg = EXCLUDED.cost_per_kg,
			flag_valuation = EXCLUDED.flag_valuation,
			flag_marketing = EXCLUDED.flag_marketing,
			flag_simulation = EXCLUDED.flag_simulation,
			init_val_valuation = EXCLUDED.init_val_valuation,
			init_val_marketing = EXCLUDED.init_val_marketing,
			init_val_simulation = EXCLUDED.init_val_simulation,
			marketing_freight_rate = EXCLUDED.marketing_freight_rate,
			marketing_anti_dumping_pct = EXCLUDED.marketing_anti_dumping_pct,
			marketing_default_value = EXCLUDED.marketing_default_value,
			valuation_flag_v2 = EXCLUDED.valuation_flag_v2,
			marketing_flag_v2 = EXCLUDED.marketing_flag_v2,
			is_backfilled = EXCLUDED.is_backfilled,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by
	`
	_, err := r.db.ExecContext(ctx, query,
		snap.ID, snap.Period, snap.GroupHeadID,
		snap.Name, nullableString(snap.Description), nullableString(snap.Colorant), nullableString(snap.CIName),
		snap.CostPercentage, snap.CostPerKg,
		snap.FlagValuation.String(), snap.FlagMarketing.String(), snap.FlagSimulation.String(),
		snap.InitValValuation, snap.InitValMarketing, snap.InitValSimulation,
		mi.FreightRate, mi.AntiDumpingPct, mi.DefaultValue,
		nullableFlagString(string(mi.ValuationFlag)), nullableFlagString(string(mi.MarketingFlag)),
		snap.IsBackfilled, snap.CreatedAt, snap.CreatedBy, snap.UpdatedAt, snap.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("upsert rm group head period snapshot: %w", err)
	}
	return nil
}

type headPeriodDTO struct {
	ID                      uuid.UUID
	Period                  string
	GroupHeadID             uuid.UUID
	Name                    string
	Description             sql.NullString
	Colorant                sql.NullString
	CIName                  sql.NullString
	CostPercentage          float64
	CostPerKg               float64
	FlagValuation           string
	FlagMarketing           string
	FlagSimulation          string
	InitValValuation        sql.NullFloat64
	InitValMarketing        sql.NullFloat64
	InitValSimulation       sql.NullFloat64
	MarketingFreightRate    sql.NullFloat64
	MarketingAntiDumpingPct sql.NullFloat64
	MarketingDefaultValue   sql.NullFloat64
	ValuationFlagV2         sql.NullString
	MarketingFlagV2         sql.NullString
	IsBackfilled            bool
	CreatedAt               time.Time
	CreatedBy               string
	UpdatedAt               sql.NullTime
	UpdatedBy               sql.NullString
}

func (d *headPeriodDTO) toEntity() (*rmgroup.HeadPeriodSnapshot, error) {
	flagV, err := rmgroup.ParseFlag(d.FlagValuation)
	if err != nil {
		return nil, fmt.Errorf("invalid flag_valuation from db: %w", err)
	}
	flagM, err := rmgroup.ParseFlag(d.FlagMarketing)
	if err != nil {
		return nil, fmt.Errorf("invalid flag_marketing from db: %w", err)
	}
	flagS, err := rmgroup.ParseFlag(d.FlagSimulation)
	if err != nil {
		return nil, fmt.Errorf("invalid flag_simulation from db: %w", err)
	}
	valFlag, err := rmgroup.ParseValuationFlag(nullStringVal(d.ValuationFlagV2))
	if err != nil {
		return nil, fmt.Errorf("invalid valuation_flag_v2 from db: %w", err)
	}
	mktFlag, err := rmgroup.ParseMarketingFlag(nullStringVal(d.MarketingFlagV2))
	if err != nil {
		return nil, fmt.Errorf("invalid marketing_flag_v2 from db: %w", err)
	}
	return &rmgroup.HeadPeriodSnapshot{
		ID:                d.ID,
		Period:            d.Period,
		GroupHeadID:       d.GroupHeadID,
		Name:              d.Name,
		Description:       nullStringVal(d.Description),
		Colorant:          nullStringVal(d.Colorant),
		CIName:            nullStringVal(d.CIName),
		CostPercentage:    d.CostPercentage,
		CostPerKg:         d.CostPerKg,
		FlagValuation:     flagV,
		FlagMarketing:     flagM,
		FlagSimulation:    flagS,
		InitValValuation:  nullFloatPtr(d.InitValValuation),
		InitValMarketing:  nullFloatPtr(d.InitValMarketing),
		InitValSimulation: nullFloatPtr(d.InitValSimulation),
		MarketingInputs: rmgroup.MarketingInputs{
			FreightRate:    nullFloatPtr(d.MarketingFreightRate),
			AntiDumpingPct: nullFloatPtr(d.MarketingAntiDumpingPct),
			DefaultValue:   nullFloatPtr(d.MarketingDefaultValue),
			ValuationFlag:  valFlag,
			MarketingFlag:  mktFlag,
		},
		IsBackfilled: d.IsBackfilled,
		CreatedAt:    d.CreatedAt,
		CreatedBy:    d.CreatedBy,
		UpdatedAt:    nullTimePtr(d.UpdatedAt),
		UpdatedBy:    nullStringPtr(d.UpdatedBy),
	}, nil
}

func scanHeadPeriodSnapshot(row *sql.Row) (*rmgroup.HeadPeriodSnapshot, error) {
	var d headPeriodDTO
	err := row.Scan(
		&d.ID, &d.Period, &d.GroupHeadID,
		&d.Name, &d.Description, &d.Colorant, &d.CIName,
		&d.CostPercentage, &d.CostPerKg,
		&d.FlagValuation, &d.FlagMarketing, &d.FlagSimulation,
		&d.InitValValuation, &d.InitValMarketing, &d.InitValSimulation,
		&d.MarketingFreightRate, &d.MarketingAntiDumpingPct, &d.MarketingDefaultValue,
		&d.ValuationFlagV2, &d.MarketingFlagV2,
		&d.IsBackfilled, &d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rmgroup.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan rm group head period snapshot: %w", err)
	}
	return d.toEntity()
}

// =============================================================================
// Detail period operations
// =============================================================================

const detailPeriodSelectSQL = `
	SELECT group_detail_period_id, period, group_detail_id,
	       market_percentage, market_value_rp, sort_order, is_active, is_dummy,
	       valuation_freight_rate, valuation_anti_dumping_pct, valuation_duty_pct,
	       valuation_transport_rate, valuation_default_value,
	       is_backfilled, created_at, created_by, updated_at, updated_by
	FROM cst_rm_group_detail_period`

// GetDetailPeriodSnapshot returns the period-scoped snapshot for (detailID, period).
// Returns rmgroup.ErrNotFound when no snapshot exists yet for that period —
// callers fall back to rmgroup.NewDetailPeriodSnapshotFromDetail.
func (r *RMGroupRepository) GetDetailPeriodSnapshot(ctx context.Context, detailID uuid.UUID, period string) (*rmgroup.DetailPeriodSnapshot, error) {
	row := r.db.QueryRowContext(ctx,
		detailPeriodSelectSQL+` WHERE group_detail_id = $1 AND period = $2`,
		detailID, period,
	)
	return scanDetailPeriodSnapshot(row)
}

// UpsertDetailPeriod writes the snapshot keyed on (period, group_detail_id).
// created_at/created_by are preserved on conflict; every other editable column
// is overwritten — mirrors UpsertHeadPeriod / rmcost_repository.go's upsertCost.
func (r *RMGroupRepository) UpsertDetailPeriod(ctx context.Context, snap *rmgroup.DetailPeriodSnapshot) error {
	vi := snap.ValuationInputs
	query := `
		INSERT INTO cst_rm_group_detail_period (
			group_detail_period_id, period, group_detail_id,
			market_percentage, market_value_rp, sort_order, is_active, is_dummy,
			valuation_freight_rate, valuation_anti_dumping_pct, valuation_duty_pct,
			valuation_transport_rate, valuation_default_value,
			is_backfilled, created_at, created_by, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
		ON CONFLICT (period, group_detail_id) DO UPDATE SET
			market_percentage = EXCLUDED.market_percentage,
			market_value_rp = EXCLUDED.market_value_rp,
			sort_order = EXCLUDED.sort_order,
			is_active = EXCLUDED.is_active,
			is_dummy = EXCLUDED.is_dummy,
			valuation_freight_rate = EXCLUDED.valuation_freight_rate,
			valuation_anti_dumping_pct = EXCLUDED.valuation_anti_dumping_pct,
			valuation_duty_pct = EXCLUDED.valuation_duty_pct,
			valuation_transport_rate = EXCLUDED.valuation_transport_rate,
			valuation_default_value = EXCLUDED.valuation_default_value,
			is_backfilled = EXCLUDED.is_backfilled,
			updated_at = EXCLUDED.updated_at,
			updated_by = EXCLUDED.updated_by
	`
	_, err := r.db.ExecContext(ctx, query,
		snap.ID, snap.Period, snap.GroupDetailID,
		snap.MarketPercentage, snap.MarketValueRp, snap.SortOrder, snap.IsActive, snap.IsDummy,
		vi.FreightRate, vi.AntiDumpingPct, vi.DutyPct, vi.TransportRate, vi.DefaultValue,
		snap.IsBackfilled, snap.CreatedAt, snap.CreatedBy, snap.UpdatedAt, snap.UpdatedBy,
	)
	if err != nil {
		return fmt.Errorf("upsert rm group detail period snapshot: %w", err)
	}
	return nil
}

type detailPeriodDTO struct {
	ID                      uuid.UUID
	Period                  string
	GroupDetailID           uuid.UUID
	MarketPercentage        sql.NullFloat64
	MarketValueRp           sql.NullFloat64
	SortOrder               int32
	IsActive                bool
	IsDummy                 bool
	ValuationFreightRate    sql.NullFloat64
	ValuationAntiDumpingPct sql.NullFloat64
	ValuationDutyPct        sql.NullFloat64
	ValuationTransportRate  sql.NullFloat64
	ValuationDefaultValue   sql.NullFloat64
	IsBackfilled            bool
	CreatedAt               time.Time
	CreatedBy               string
	UpdatedAt               sql.NullTime
	UpdatedBy               sql.NullString
}

func (d *detailPeriodDTO) toEntity() *rmgroup.DetailPeriodSnapshot {
	return &rmgroup.DetailPeriodSnapshot{
		ID:               d.ID,
		Period:           d.Period,
		GroupDetailID:    d.GroupDetailID,
		MarketPercentage: nullFloatPtr(d.MarketPercentage),
		MarketValueRp:    nullFloatPtr(d.MarketValueRp),
		SortOrder:        d.SortOrder,
		IsActive:         d.IsActive,
		IsDummy:          d.IsDummy,
		ValuationInputs: rmgroup.ValuationInputs{
			FreightRate:    nullFloatPtr(d.ValuationFreightRate),
			AntiDumpingPct: nullFloatPtr(d.ValuationAntiDumpingPct),
			DutyPct:        nullFloatPtr(d.ValuationDutyPct),
			TransportRate:  nullFloatPtr(d.ValuationTransportRate),
			DefaultValue:   nullFloatPtr(d.ValuationDefaultValue),
		},
		IsBackfilled: d.IsBackfilled,
		CreatedAt:    d.CreatedAt,
		CreatedBy:    d.CreatedBy,
		UpdatedAt:    nullTimePtr(d.UpdatedAt),
		UpdatedBy:    nullStringPtr(d.UpdatedBy),
	}
}

func scanDetailPeriodSnapshot(row *sql.Row) (*rmgroup.DetailPeriodSnapshot, error) {
	var d detailPeriodDTO
	err := row.Scan(
		&d.ID, &d.Period, &d.GroupDetailID,
		&d.MarketPercentage, &d.MarketValueRp, &d.SortOrder, &d.IsActive, &d.IsDummy,
		&d.ValuationFreightRate, &d.ValuationAntiDumpingPct, &d.ValuationDutyPct,
		&d.ValuationTransportRate, &d.ValuationDefaultValue,
		&d.IsBackfilled, &d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rmgroup.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan rm group detail period snapshot: %w", err)
	}
	return d.toEntity(), nil
}

// =============================================================================
// LatestSyncPeriod
// =============================================================================

// LatestSyncPeriod returns the most recent period known to the system.
//
// Verified against the actual RPC chain before implementing (per Task B1-6):
// the frontend's useSyncPeriods hook (goapps-frontend/src/hooks/finance/use-oracle-sync.ts)
// calls GET /api/v1/finance/oracle-sync/periods, which the BFF route forwards to
// OracleSyncHandler.ListSyncPeriods -> oraclesync.ListPeriodsHandler ->
// syncdata.PostgresTargetRepository.GetDistinctPeriods, which queries
// cst_item_cons_stk_po — the Oracle stock/PO staging table for the unrelated
// Oracle-sync-jobs feature, NOT the RM costing period timeline.
//
// The period that matters for "should this update also write through to the
// anchor row" is the RM costing timeline, i.e. rmcost.Repository's
// ListDistinctPeriods, which queries cst_rm_cost ordered by period DESC. This
// method duplicates that single query rather than importing rmcost's
// repository: it is infrastructure-layer SQL (not a domain-layer import), so
// there is no layering violation, but keeping the two repositories
// independent avoids a cross-repository behavioral coupling for a one-line
// query. If cst_rm_cost has no rows yet, returns rmgroup.ErrNotFound.
func (r *RMGroupRepository) LatestSyncPeriod(ctx context.Context) (string, error) {
	var period string
	err := r.db.QueryRowContext(ctx,
		`SELECT period FROM cst_rm_cost ORDER BY period DESC LIMIT 1`,
	).Scan(&period)
	if errors.Is(err, sql.ErrNoRows) {
		return "", rmgroup.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("latest sync period: %w", err)
	}
	return period, nil
}
