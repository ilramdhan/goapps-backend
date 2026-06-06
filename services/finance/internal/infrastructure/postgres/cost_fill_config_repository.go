package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
)

// CostFillConfigRepository implements domain.ConfigRepository for the three config tiers.
type CostFillConfigRepository struct{ db *DB }

// NewCostFillConfigRepository constructs the repo.
func NewCostFillConfigRepository(db *DB) *CostFillConfigRepository {
	return &CostFillConfigRepository{db: db}
}

var _ domain.ConfigRepository = (*CostFillConfigRepository)(nil)

// UpsertGlobal deactivates any prior active row for the level then inserts a new one.
func (r *CostFillConfigRepository) UpsertGlobal(ctx context.Context, c *domain.Config, actor string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert global: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("rollback upsert global config")
		}
	}()
	if _, err = tx.ExecContext(ctx,
		`UPDATE cost_level_assignment_config
		    SET clac_is_active=false, clac_updated_at=NOW(), clac_updated_by=$2
		  WHERE clac_route_level=$1 AND clac_is_active=true`,
		c.RouteLevel, actor); err != nil {
		return fmt.Errorf("deactivate prior global: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO cost_level_assignment_config
		   (clac_route_level, clac_filler_type, clac_filler_value,
		    clac_approver_type, clac_approver_value,
		    clac_reapprove_on_change, clac_sla_fill_hours, clac_sla_approve_hours,
		    clac_created_by, clac_updated_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)`,
		c.RouteLevel,
		derefStr(c.FillerType), derefStr(c.FillerValue),
		c.ApproverType, c.ApproverValue,
		derefBool(c.ReapproveOnChange),
		derefI32(c.SLAFillHours, 48),
		derefI32(c.SLAApproveHours, 24),
		actor,
	); err != nil {
		return fmt.Errorf("insert global config: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert global: %w", err)
	}
	return nil
}

// DeleteGlobal marks the active global config for a level as inactive.
func (r *CostFillConfigRepository) DeleteGlobal(ctx context.Context, routeLevel int32) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cost_level_assignment_config
		    SET clac_is_active=false, clac_updated_at=NOW()
		  WHERE clac_route_level=$1 AND clac_is_active=true`,
		routeLevel)
	if err != nil {
		return fmt.Errorf("delete global config level %d: %w", routeLevel, err)
	}
	return nil
}

// ListGlobal returns all active global configs ordered by route level.
func (r *CostFillConfigRepository) ListGlobal(ctx context.Context) ([]*domain.Config, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT clac_config_id, clac_route_level,
		        clac_filler_type, clac_filler_value,
		        clac_approver_type, clac_approver_value,
		        clac_reapprove_on_change, clac_sla_fill_hours, clac_sla_approve_hours
		   FROM cost_level_assignment_config
		  WHERE clac_is_active=true
		  ORDER BY clac_route_level`)
	if err != nil {
		return nil, fmt.Errorf("list global configs: %w", err)
	}
	defer rows.Close()
	var out []*domain.Config
	for rows.Next() {
		c, scanErr := scanGlobalConfig(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, c)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err list global: %w", err)
	}
	return out, nil
}

// GetGlobal returns the active global config for a level, or ErrConfigNotFound.
func (r *CostFillConfigRepository) GetGlobal(ctx context.Context, routeLevel int32) (*domain.Config, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT clac_config_id, clac_route_level,
		        clac_filler_type, clac_filler_value,
		        clac_approver_type, clac_approver_value,
		        clac_reapprove_on_change, clac_sla_fill_hours, clac_sla_approve_hours
		   FROM cost_level_assignment_config
		  WHERE clac_route_level=$1 AND clac_is_active=true`,
		routeLevel)
	c, err := scanGlobalConfigRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrConfigNotFound
	}
	return c, err
}

// UpsertProduct inserts or updates a per-product level override.
func (r *CostFillConfigRepository) UpsertProduct(ctx context.Context, c *domain.Config, actor string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cost_product_level_assignment
		   (cpla_product_sys_id, cpla_route_level,
		    cpla_filler_type, cpla_filler_value,
		    cpla_approver_type, cpla_approver_value,
		    cpla_reapprove_on_change, cpla_sla_fill_hours, cpla_sla_approve_hours,
		    cpla_created_by, cpla_updated_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		 ON CONFLICT (cpla_product_sys_id, cpla_route_level) DO UPDATE SET
		   cpla_filler_type=EXCLUDED.cpla_filler_type,
		   cpla_filler_value=EXCLUDED.cpla_filler_value,
		   cpla_approver_type=EXCLUDED.cpla_approver_type,
		   cpla_approver_value=EXCLUDED.cpla_approver_value,
		   cpla_reapprove_on_change=EXCLUDED.cpla_reapprove_on_change,
		   cpla_sla_fill_hours=EXCLUDED.cpla_sla_fill_hours,
		   cpla_sla_approve_hours=EXCLUDED.cpla_sla_approve_hours,
		   cpla_updated_at=NOW(), cpla_updated_by=EXCLUDED.cpla_updated_by`,
		c.ProductSysID, c.RouteLevel,
		c.FillerType, c.FillerValue,
		c.ApproverType, c.ApproverValue,
		c.ReapproveOnChange, c.SLAFillHours, c.SLAApproveHours,
		actor)
	if err != nil {
		return fmt.Errorf("upsert product override: %w", err)
	}
	return nil
}

// GetProduct returns the product override for (product, level) or nil if none.
func (r *CostFillConfigRepository) GetProduct(ctx context.Context, productSysID int64, routeLevel int32) (*domain.Config, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT cpla_assignment_id, cpla_route_level,
		        cpla_filler_type, cpla_filler_value,
		        cpla_approver_type, cpla_approver_value,
		        cpla_reapprove_on_change, cpla_sla_fill_hours, cpla_sla_approve_hours
		   FROM cost_product_level_assignment
		  WHERE cpla_product_sys_id=$1 AND cpla_route_level=$2`,
		productSysID, routeLevel)
	var (
		id, level                   int64
		fillerType, fillerValue     sql.NullString
		approverType, approverValue sql.NullString
		reapprove                   sql.NullBool
		slaFill, slaApprove         sql.NullInt32
	)
	err := row.Scan(&id, &level,
		&fillerType, &fillerValue,
		&approverType, &approverValue,
		&reapprove, &slaFill, &slaApprove)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil,nil means "no override row"
	}
	if err != nil {
		return nil, fmt.Errorf("get product override: %w", err)
	}
	return &domain.Config{
		ConfigID:          id,
		Tier:              domain.TierProduct,
		RouteLevel:        int32(level), //nolint:gosec // PK level fits int32
		ProductSysID:      productSysID,
		FillerType:        nullStrPtr(fillerType),
		FillerValue:       nullStrPtr(fillerValue),
		ApproverType:      nullStrPtr(approverType),
		ApproverValue:     nullStrPtr(approverValue),
		ReapproveOnChange: nullBoolPtr(reapprove),
		SLAFillHours:      nullI32Ptr(slaFill),
		SLAApproveHours:   nullI32Ptr(slaApprove),
	}, nil
}

// UpsertRequest inserts or updates a per-request level override.
func (r *CostFillConfigRepository) UpsertRequest(ctx context.Context, c *domain.Config, actor string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cost_request_level_assignment
		   (crla_request_id, crla_route_level,
		    crla_filler_type, crla_filler_value,
		    crla_approver_type, crla_approver_value,
		    crla_reapprove_on_change, crla_sla_fill_hours, crla_sla_approve_hours,
		    crla_created_by, crla_updated_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
		 ON CONFLICT (crla_request_id, crla_route_level) DO UPDATE SET
		   crla_filler_type=EXCLUDED.crla_filler_type,
		   crla_filler_value=EXCLUDED.crla_filler_value,
		   crla_approver_type=EXCLUDED.crla_approver_type,
		   crla_approver_value=EXCLUDED.crla_approver_value,
		   crla_reapprove_on_change=EXCLUDED.crla_reapprove_on_change,
		   crla_sla_fill_hours=EXCLUDED.crla_sla_fill_hours,
		   crla_sla_approve_hours=EXCLUDED.crla_sla_approve_hours,
		   crla_updated_at=NOW(), crla_updated_by=EXCLUDED.crla_updated_by`,
		c.RequestID, c.RouteLevel,
		c.FillerType, c.FillerValue,
		c.ApproverType, c.ApproverValue,
		c.ReapproveOnChange, c.SLAFillHours, c.SLAApproveHours,
		actor)
	if err != nil {
		return fmt.Errorf("upsert request override: %w", err)
	}
	return nil
}

// GetRequest returns the request override for (request, level) or nil if none.
func (r *CostFillConfigRepository) GetRequest(ctx context.Context, requestID int64, routeLevel int32) (*domain.Config, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT crla_assignment_id, crla_route_level,
		        crla_filler_type, crla_filler_value,
		        crla_approver_type, crla_approver_value,
		        crla_reapprove_on_change, crla_sla_fill_hours, crla_sla_approve_hours
		   FROM cost_request_level_assignment
		  WHERE crla_request_id=$1 AND crla_route_level=$2`,
		requestID, routeLevel)
	var (
		id, level                   int64
		fillerType, fillerValue     sql.NullString
		approverType, approverValue sql.NullString
		reapprove                   sql.NullBool
		slaFill, slaApprove         sql.NullInt32
	)
	err := row.Scan(&id, &level,
		&fillerType, &fillerValue,
		&approverType, &approverValue,
		&reapprove, &slaFill, &slaApprove)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil,nil means "no override row"
	}
	if err != nil {
		return nil, fmt.Errorf("get request override: %w", err)
	}
	return &domain.Config{
		ConfigID:          id,
		Tier:              domain.TierRequest,
		RouteLevel:        int32(level), //nolint:gosec // PK level fits int32
		RequestID:         requestID,
		FillerType:        nullStrPtr(fillerType),
		FillerValue:       nullStrPtr(fillerValue),
		ApproverType:      nullStrPtr(approverType),
		ApproverValue:     nullStrPtr(approverValue),
		ReapproveOnChange: nullBoolPtr(reapprove),
		SLAFillHours:      nullI32Ptr(slaFill),
		SLAApproveHours:   nullI32Ptr(slaApprove),
	}, nil
}

// --- helpers ---

type globalConfigScanner interface {
	Scan(dest ...any) error
}

func scanGlobalConfig(rows *sql.Rows) (*domain.Config, error) {
	return scanGlobalConfigRow(rows)
}

func scanGlobalConfigRow(s globalConfigScanner) (*domain.Config, error) {
	var (
		id, level                   int64
		fillerType, fillerValue     string
		approverType, approverValue sql.NullString
		reapprove                   bool
		slaFill, slaApprove         int32
	)
	if err := s.Scan(&id, &level,
		&fillerType, &fillerValue,
		&approverType, &approverValue,
		&reapprove, &slaFill, &slaApprove); err != nil {
		return nil, fmt.Errorf("scan global config: %w", err)
	}
	lvl := int32(level) //nolint:gosec // route level fits int32
	return &domain.Config{
		ConfigID:          id,
		Tier:              domain.TierGlobal,
		RouteLevel:        lvl,
		FillerType:        &fillerType,
		FillerValue:       &fillerValue,
		ApproverType:      nullStrPtr(approverType),
		ApproverValue:     nullStrPtr(approverValue),
		ReapproveOnChange: &reapprove,
		SLAFillHours:      &slaFill,
		SLAApproveHours:   &slaApprove,
	}, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefBool(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func derefI32(v *int32, def int32) int32 {
	if v == nil {
		return def
	}
	return *v
}

func nullStrPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func nullBoolPtr(n sql.NullBool) *bool {
	if !n.Valid {
		return nil
	}
	return &n.Bool
}

func nullI32Ptr(n sql.NullInt32) *int32 {
	if !n.Valid {
		return nil
	}
	return &n.Int32
}
