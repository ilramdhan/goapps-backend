// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// RMGroupRepository implements rmgroup.Repository using PostgreSQL.
type RMGroupRepository struct {
	db *DB
}

// NewRMGroupRepository creates a new RMGroupRepository instance.
func NewRMGroupRepository(db *DB) *RMGroupRepository {
	return &RMGroupRepository{db: db}
}

// =============================================================================
// Head operations
// =============================================================================

// CreateHead persists a new Head row and writes a CREATE audit row in the same tx.
func (r *RMGroupRepository) CreateHead(ctx context.Context, head *rmgroup.Head) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		query := `
			INSERT INTO cst_rm_group_head (
				group_head_id, group_code, group_name, description, colourant, ci_name,
				cost_percentage, cost_per_kg,
				flag_valuation, flag_marketing, flag_simulation,
				init_val_valuation, init_val_marketing, init_val_simulation,
				is_active, created_at, created_by,
				marketing_freight_rate, marketing_anti_dumping_pct, marketing_default_value,
				valuation_flag, marketing_flag
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		`
		mi := head.MarketingInputs()
		_, err := tx.ExecContext(ctx, query,
			head.ID(), head.Code().String(), head.Name(), head.Description(),
			nullableString(head.Colorant()), nullableString(head.CIName()),
			head.CostPercentage(), head.CostPerKg(),
			head.FlagValuation().String(), head.FlagMarketing().String(), head.FlagSimulation().String(),
			head.InitValValuation(), head.InitValMarketing(), head.InitValSimulation(),
			head.IsActive(), head.CreatedAt(), head.CreatedBy(),
			mi.FreightRate, mi.AntiDumpingPct, mi.DefaultValue,
			nullableFlagString(string(mi.ValuationFlag)), nullableFlagString(string(mi.MarketingFlag)),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return rmgroup.ErrCodeAlreadyExists
			}
			return fmt.Errorf("create rm group head: %w", err)
		}
		return insertHeadAudit(ctx, tx, head, auditActionCreate, head.CreatedBy())
	})
}

// GetHeadByID retrieves a head by ID.
func (r *RMGroupRepository) GetHeadByID(ctx context.Context, id uuid.UUID) (*rmgroup.Head, error) {
	return r.scanHead(r.db.QueryRowContext(ctx, headSelectSQL+` WHERE group_head_id = $1 AND deleted_at IS NULL`, id))
}

// GetHeadByCode retrieves a head by its unique code.
func (r *RMGroupRepository) GetHeadByCode(ctx context.Context, code rmgroup.Code) (*rmgroup.Head, error) {
	return r.scanHead(r.db.QueryRowContext(ctx, headSelectSQL+` WHERE group_code = $1 AND deleted_at IS NULL`, code.String()))
}

// ListHeads returns a page of heads plus the total count.
func (r *RMGroupRepository) ListHeads(ctx context.Context, filter rmgroup.ListFilter) ([]*rmgroup.Head, int64, error) {
	filter.Validate()

	base := ` WHERE deleted_at IS NULL`
	args := []any{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (group_code ILIKE $%d OR group_name ILIKE $%d OR description ILIKE $%d OR colourant ILIKE $%d OR ci_name ILIKE $%d)`,
			argIdx, argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}
	if filter.Flag != "" {
		base += fmt.Sprintf(` AND (flag_valuation = $%d OR flag_marketing = $%d OR flag_simulation = $%d)`, argIdx, argIdx, argIdx)
		args = append(args, filter.Flag.String())
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cst_rm_group_head`+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rm group heads: %w", err)
	}

	orderCol := map[string]string{
		"code":       "group_code",
		"name":       "group_name",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}[filter.SortBy]
	if orderCol == "" {
		orderCol = "group_code"
	}
	orderDir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		orderDir = sortDESC
	}

	selectQuery := headSelectSQL + " " + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, orderDir, argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list rm group heads: %w", err)
	}
	defer closeRows(rows)

	var heads []*rmgroup.Head
	for rows.Next() {
		head, err := r.scanHeadRow(rows)
		if err != nil {
			return nil, 0, err
		}
		heads = append(heads, head)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rm group heads: %w", err)
	}
	return heads, total, nil
}

// ListAllHeads returns every non-deleted head matching the active filter,
// ordered by group_code. No pagination — intended for export.
func (r *RMGroupRepository) ListAllHeads(ctx context.Context, activeFilter *bool) ([]*rmgroup.Head, error) {
	base := ` WHERE deleted_at IS NULL`
	args := []any{}
	if activeFilter != nil {
		base += ` AND is_active = $1`
		args = append(args, *activeFilter)
	}
	q := headSelectSQL + " " + base + ` ORDER BY group_code ASC`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list all rm group heads: %w", err)
	}
	defer closeRows(rows)

	var out []*rmgroup.Head
	for rows.Next() {
		h, err := r.scanHeadRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all rm group heads: %w", err)
	}
	return out, nil
}

// UpdateHead persists changes to an existing head and writes an UPDATE audit row in the same tx.
func (r *RMGroupRepository) UpdateHead(ctx context.Context, head *rmgroup.Head) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		query := `
			UPDATE cst_rm_group_head SET
				group_name = $2, description = $3, colourant = $4, ci_name = $5,
				cost_percentage = $6, cost_per_kg = $7,
				flag_valuation = $8, flag_marketing = $9, flag_simulation = $10,
				init_val_valuation = $11, init_val_marketing = $12, init_val_simulation = $13,
				is_active = $14, updated_at = $15, updated_by = $16,
				marketing_freight_rate = $17, marketing_anti_dumping_pct = $18, marketing_default_value = $19,
				valuation_flag = $20, marketing_flag = $21
			WHERE group_head_id = $1 AND deleted_at IS NULL
		`
		mi := head.MarketingInputs()
		res, err := tx.ExecContext(ctx, query,
			head.ID(), head.Name(), head.Description(),
			nullableString(head.Colorant()), nullableString(head.CIName()),
			head.CostPercentage(), head.CostPerKg(),
			head.FlagValuation().String(), head.FlagMarketing().String(), head.FlagSimulation().String(),
			head.InitValValuation(), head.InitValMarketing(), head.InitValSimulation(),
			head.IsActive(), head.UpdatedAt(), head.UpdatedBy(),
			mi.FreightRate, mi.AntiDumpingPct, mi.DefaultValue,
			nullableFlagString(string(mi.ValuationFlag)), nullableFlagString(string(mi.MarketingFlag)),
		)
		if err != nil {
			return fmt.Errorf("update rm group head: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			return rmgroup.ErrNotFound
		}
		return insertHeadAudit(ctx, tx, head, auditActionUpdate, derefString(head.UpdatedBy()))
	})
}

// SoftDeleteHead marks the head and all of its active details as deleted in a single tx,
// and writes a DELETE audit row for the head and for each affected detail.
func (r *RMGroupRepository) SoftDeleteHead(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		head, err := r.scanHead(tx.QueryRowContext(ctx, headSelectSQL+` WHERE group_head_id = $1 AND deleted_at IS NULL`, id))
		if err != nil {
			return err
		}
		details, err := r.listDetails(ctx,
			detailSelectSQL+` WHERE group_head_id=$1 AND deleted_at IS NULL`, id)
		if err != nil {
			return err
		}

		now := time.Now()
		res, err := tx.ExecContext(ctx, `
			UPDATE cst_rm_group_head SET deleted_at=$2, deleted_by=$3, is_active=false
			WHERE group_head_id=$1 AND deleted_at IS NULL
		`, id, now, deletedBy)
		if err != nil {
			return fmt.Errorf("soft-delete head: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			return rmgroup.ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE cst_rm_group_detail SET deleted_at=$2, deleted_by=$3, is_active=false
			WHERE group_head_id=$1 AND deleted_at IS NULL
		`, id, now, deletedBy); err != nil {
			return fmt.Errorf("soft-delete details: %w", err)
		}

		if err := insertHeadAudit(ctx, tx, head, auditActionDelete, deletedBy); err != nil {
			return err
		}
		for _, d := range details {
			if err := insertDetailAuditDelete(ctx, tx, d, deletedBy); err != nil {
				return err
			}
		}
		return nil
	})
}

// derefString returns the value pointed to by s, or "" if s is nil. Used for
// audit changedBy values where the updater/deleter field is an optional *string.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ExistsHeadByCode reports whether a non-deleted head with this code exists.
func (r *RMGroupRepository) ExistsHeadByCode(ctx context.Context, code rmgroup.Code) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM cst_rm_group_head WHERE group_code=$1 AND deleted_at IS NULL)`,
		code.String()).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists head by code: %w", err)
	}
	return exists, nil
}

// ExistsHeadByID reports whether a non-deleted head with this ID exists.
func (r *RMGroupRepository) ExistsHeadByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM cst_rm_group_head WHERE group_head_id=$1 AND deleted_at IS NULL)`,
		id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists head by id: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Head scanning helpers
// =============================================================================

const headSelectSQL = `
	SELECT group_head_id, group_code, group_name, description, colourant, ci_name,
	       cost_percentage, cost_per_kg,
	       flag_valuation, flag_marketing, flag_simulation,
	       init_val_valuation, init_val_marketing, init_val_simulation,
	       is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by,
	       marketing_freight_rate, marketing_anti_dumping_pct, marketing_default_value,
	       valuation_flag, marketing_flag
	FROM cst_rm_group_head`

type headDTO struct {
	ID                       uuid.UUID
	Code                     string
	Name                     string
	Description              sql.NullString
	Colorant                 sql.NullString
	CIName                   sql.NullString
	CostPercentage           float64
	CostPerKg                float64
	FlagValuation            string
	FlagMarketing            string
	FlagSimulation           string
	InitValValuation         sql.NullFloat64
	InitValMarketing         sql.NullFloat64
	InitValSimulation        sql.NullFloat64
	IsActive                 bool
	CreatedAt                time.Time
	CreatedBy                string
	UpdatedAt                sql.NullTime
	UpdatedBy                sql.NullString
	DeletedAt                sql.NullTime
	DeletedBy                sql.NullString
	MarketingFreightRate     sql.NullFloat64
	MarketingAntiDumpingPct  sql.NullFloat64
	MarketingDefaultValue    sql.NullFloat64
	ValuationFlagV2          sql.NullString
	MarketingFlagV2          sql.NullString
}

func (d *headDTO) toEntity() (*rmgroup.Head, error) {
	code, err := rmgroup.NewCode(d.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}
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
	head := rmgroup.ReconstructHead(
		d.ID, code, d.Name,
		nullStringVal(d.Description), nullStringVal(d.Colorant), nullStringVal(d.CIName),
		d.CostPercentage, d.CostPerKg,
		flagV, flagM, flagS,
		nullFloatPtr(d.InitValValuation), nullFloatPtr(d.InitValMarketing), nullFloatPtr(d.InitValSimulation),
		d.IsActive, d.CreatedAt, d.CreatedBy,
		nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
		nullTimePtr(d.DeletedAt), nullStringPtr(d.DeletedBy),
	)
	valFlag, err := rmgroup.ParseValuationFlag(nullStringVal(d.ValuationFlagV2))
	if err != nil {
		return nil, fmt.Errorf("invalid valuation_flag from db: %w", err)
	}
	mktFlag, err := rmgroup.ParseMarketingFlag(nullStringVal(d.MarketingFlagV2))
	if err != nil {
		return nil, fmt.Errorf("invalid marketing_flag from db: %w", err)
	}
	if err := head.AttachMarketingInputs(rmgroup.MarketingInputs{
		FreightRate:    nullFloatPtr(d.MarketingFreightRate),
		AntiDumpingPct: nullFloatPtr(d.MarketingAntiDumpingPct),
		DefaultValue:   nullFloatPtr(d.MarketingDefaultValue),
		ValuationFlag:  valFlag,
		MarketingFlag:  mktFlag,
	}); err != nil {
		return nil, fmt.Errorf("attach marketing inputs from db: %w", err)
	}
	return head, nil
}

func (r *RMGroupRepository) scanHead(row *sql.Row) (*rmgroup.Head, error) {
	var d headDTO
	err := row.Scan(
		&d.ID, &d.Code, &d.Name, &d.Description, &d.Colorant, &d.CIName,
		&d.CostPercentage, &d.CostPerKg,
		&d.FlagValuation, &d.FlagMarketing, &d.FlagSimulation,
		&d.InitValValuation, &d.InitValMarketing, &d.InitValSimulation,
		&d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		&d.MarketingFreightRate, &d.MarketingAntiDumpingPct, &d.MarketingDefaultValue,
		&d.ValuationFlagV2, &d.MarketingFlagV2,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rmgroup.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan rm group head: %w", err)
	}
	return d.toEntity()
}

func (r *RMGroupRepository) scanHeadRow(rows *sql.Rows) (*rmgroup.Head, error) {
	var d headDTO
	err := rows.Scan(
		&d.ID, &d.Code, &d.Name, &d.Description, &d.Colorant, &d.CIName,
		&d.CostPercentage, &d.CostPerKg,
		&d.FlagValuation, &d.FlagMarketing, &d.FlagSimulation,
		&d.InitValValuation, &d.InitValMarketing, &d.InitValSimulation,
		&d.IsActive, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		&d.MarketingFreightRate, &d.MarketingAntiDumpingPct, &d.MarketingDefaultValue,
		&d.ValuationFlagV2, &d.MarketingFlagV2,
	)
	if err != nil {
		return nil, fmt.Errorf("scan rm group head row: %w", err)
	}
	return d.toEntity()
}

// =============================================================================
// Shared helpers (used by head + detail repo + rmcost repo).
// =============================================================================

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableFlagString stores AUTO/empty as NULL so the DB column is NULL when
// the user has not picked an explicit flag. Other values (CR/SL/SP/...) round-trip.
func nullableFlagString(s string) sql.NullString {
	if s == "" || s == "AUTO" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullStringVal(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func nullStringPtr(v sql.NullString) *string {
	if v.Valid {
		s := v.String
		return &s
	}
	return nil
}

func nullFloatPtr(v sql.NullFloat64) *float64 {
	if v.Valid {
		f := v.Float64
		return &f
	}
	return nil
}
