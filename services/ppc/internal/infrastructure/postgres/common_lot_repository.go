package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/commonlot"
)

// CommonLotRepository is the PostgreSQL implementation of commonlot.Repository.
// common_lot holds the new ERP identity; common_lot_component holds the folded-in
// original lots.
type CommonLotRepository struct {
	db *DB
}

// NewCommonLotRepository creates a new CommonLotRepository.
func NewCommonLotRepository(db *DB) *CommonLotRepository {
	return &CommonLotRepository{db: db}
}

// Create inserts a common lot and its components in a single transaction, setting
// the generated id on the aggregate. A duplicate lot number yields
// commonlot.ErrAlreadyExists.
func (r *CommonLotRepository) Create(ctx context.Context, lot *commonlot.CommonLot) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		const lotInsert = `
			INSERT INTO common_lot (cl_lot_no, cl_item_code, cl_shade_code, cl_erp_grade_code)
			VALUES ($1, $2, $3, $4)
			RETURNING cl_id`
		var lotID int64
		err := tx.QueryRowContext(ctx, lotInsert,
			lot.LotNo(), nullableText(lot.ItemCode()), nullableText(lot.ShadeCode()), nullableText(lot.ErpGradeCode()),
		).Scan(&lotID)
		if err != nil {
			if isUniqueViolation(err) {
				return commonlot.ErrAlreadyExists
			}
			return fmt.Errorf("insert common_lot: %w", err)
		}
		lot.SetID(lotID)

		for _, comp := range lot.Components() {
			if err := insertCommonLotComponent(ctx, tx, lotID, comp); err != nil {
				return err
			}
		}
		return nil
	})
}

// insertCommonLotComponent inserts one component line for a common lot.
func insertCommonLotComponent(ctx context.Context, tx *sql.Tx, commonLotID int64, comp commonlot.Component) error {
	const query = `
		INSERT INTO common_lot_component (
			clc_common_lot_id, clc_original_lot_no, clc_original_shade_code,
			clc_bobbin_count, clc_qty_kg)
		VALUES ($1, $2, $3, $4::INT, $5::DECIMAL)`
	if _, err := tx.ExecContext(ctx, query,
		commonLotID, comp.OriginalLotNo(), nullableText(comp.OriginalShadeCode()),
		comp.BobbinCount(), comp.QtyKg(),
	); err != nil {
		return fmt.Errorf("insert common_lot_component %s: %w", comp.OriginalLotNo(), err)
	}
	return nil
}

const commonLotColumns = `
	cl_id, cl_lot_no, cl_item_code, cl_shade_code, cl_erp_grade_code, cl_created_at`

// GetByID loads a common lot and its components.
func (r *CommonLotRepository) GetByID(ctx context.Context, id int64) (*commonlot.CommonLot, error) {
	query := `SELECT ` + commonLotColumns + ` FROM common_lot WHERE cl_id = $1`
	lot, err := r.scanCommonLot(r.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, commonlot.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get common_lot %d: %w", id, err)
	}
	comps, err := r.loadComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	return commonlot.ReconstructCommonLot(lot.id, lot.lotNo, lot.itemCode, lot.shadeCode,
		lot.erpGradeCode, lot.createdAt, comps), nil
}

// List returns common lots matching the filter plus the total count. Components
// are loaded per row so the list view can show the fold-in breakdown.
func (r *CommonLotRepository) List(ctx context.Context, filter commonlot.Filter) ([]*commonlot.CommonLot, int64, error) {
	where, args := buildCommonLotFilter(filter)
	countQuery := `SELECT COUNT(*) FROM common_lot` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count common_lot: %w", err)
	}

	orderBy := commonLotOrderBy(filter.SortBy, filter.SortOrder)
	limit, offset := pageBounds(filter.Page, filter.PageSize)
	listQuery := `SELECT ` + commonLotColumns + ` FROM common_lot` + where +
		fmt.Sprintf(` ORDER BY %s LIMIT %d OFFSET %d`, orderBy, limit, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list common_lot: %w", err)
	}
	defer closeRows(rows)

	var scanned []scannedCommonLot
	for rows.Next() {
		lot, scanErr := r.scanCommonLot(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan common_lot: %w", scanErr)
		}
		scanned = append(scanned, lot)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate common_lot: %w", err)
	}

	lots := make([]*commonlot.CommonLot, 0, len(scanned))
	for _, lot := range scanned {
		comps, err := r.loadComponents(ctx, lot.id)
		if err != nil {
			return nil, 0, err
		}
		lots = append(lots, commonlot.ReconstructCommonLot(lot.id, lot.lotNo, lot.itemCode,
			lot.shadeCode, lot.erpGradeCode, lot.createdAt, comps))
	}
	return lots, total, nil
}

// scannedCommonLot holds the flat columns of a common_lot row before rebuild.
type scannedCommonLot struct {
	id           int64
	lotNo        string
	itemCode     string
	shadeCode    string
	erpGradeCode string
	createdAt    time.Time
}

// scanCommonLot scans a flat common_lot row, coalescing nullable text.
func (r *CommonLotRepository) scanCommonLot(s rowScanner) (scannedCommonLot, error) {
	var (
		lot                               scannedCommonLot
		itemCode, shadeCode, erpGradeCode sql.NullString
	)
	err := s.Scan(&lot.id, &lot.lotNo, &itemCode, &shadeCode, &erpGradeCode, &lot.createdAt)
	if err != nil {
		return scannedCommonLot{}, err
	}
	lot.itemCode = nullString(itemCode)
	lot.shadeCode = nullString(shadeCode)
	lot.erpGradeCode = nullString(erpGradeCode)
	return lot, nil
}

// loadComponents reads all component lines for a common lot ordered by id.
func (r *CommonLotRepository) loadComponents(ctx context.Context, commonLotID int64) ([]commonlot.Component, error) {
	const query = `
		SELECT clc_id, clc_common_lot_id, clc_original_lot_no, clc_original_shade_code,
			clc_bobbin_count, clc_qty_kg
		FROM common_lot_component WHERE clc_common_lot_id = $1 ORDER BY clc_id`
	rows, err := r.db.QueryContext(ctx, query, commonLotID)
	if err != nil {
		return nil, fmt.Errorf("list common_lot_component (lot=%d): %w", commonLotID, err)
	}
	defer closeRows(rows)

	var comps []commonlot.Component
	for rows.Next() {
		var (
			id, clID    int64
			originalLot string
			shade       sql.NullString
			bobbins     sql.NullInt64
			qty         sql.NullFloat64
		)
		if err := rows.Scan(&id, &clID, &originalLot, &shade, &bobbins, &qty); err != nil {
			return nil, fmt.Errorf("scan common_lot_component: %w", err)
		}
		comps = append(comps, commonlot.ReconstructComponent(
			id, clID, strings.TrimSpace(originalLot), nullString(shade),
			safeInt64ToInt32(bobbins.Int64), qty.Float64,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate common_lot_component: %w", err)
	}
	return comps, nil
}

// buildCommonLotFilter builds the WHERE clause and args for the list query.
func buildCommonLotFilter(filter commonlot.Filter) (string, []any) {
	var conds []string
	var args []any
	idx := 1
	if s := strings.TrimSpace(filter.Search); s != "" {
		conds = append(conds, fmt.Sprintf("cl_lot_no ILIKE $%d", idx))
		args = append(args, "%"+s+"%")
		idx++
	}
	if filter.ItemCode != "" {
		conds = append(conds, fmt.Sprintf("cl_item_code = $%d", idx))
		args = append(args, filter.ItemCode)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// commonLotOrderBy maps a frontend sort field to a safe ORDER BY clause.
func commonLotOrderBy(sortBy, sortOrder string) string {
	col := "cl_id"
	switch sortBy {
	case "lot_no":
		col = "cl_lot_no"
	case "item_code":
		col = "cl_item_code"
	case "created_at":
		col = "cl_created_at"
	}
	dir := sortDESC
	if strings.EqualFold(sortOrder, "asc") {
		dir = sortASC
	}
	return col + " " + dir + ", cl_id " + sortDESC
}
