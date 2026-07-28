package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// DemandRepository implements demand.Repository using PostgreSQL.
type DemandRepository struct {
	db *DB
}

// NewDemandRepository creates a new DemandRepository.
func NewDemandRepository(db *DB) *DemandRepository {
	return &DemandRepository{db: db}
}

var _ demand.Repository = (*DemandRepository)(nil)

const demandColumns = `pd_id, pd_type, pd_sub_type, pd_source, pd_carry_action, pd_cpm_product_sys_id,
	pd_qty_original, pd_qty_remaining, pd_deadline, pd_customer_id, pd_contract_no, pd_contract_date,
	pd_stuff_advance_no, pd_incoterm, pd_lc_status, pd_grade_requirement, pd_ax_min_pct, pd_am_max_pct,
	pd_carry_from_id, pd_sos_ref, pd_status, pd_month, pd_confirmed_by, pd_confirmed_at,
	pd_created_by, pd_created_at, pd_updated_at, pd_shade_code, pd_shade_name,
	pd_product_link_reason`

// demandProductArg maps the domain's 0 sentinel onto a real SQL NULL. The
// chk_pd_product_link CHECK pairs NULL with PENDING_PRODUCT_LINK, so writing 0
// for an unlinked demand would be rejected by the database.
func demandProductArg(sysID int64) sql.NullInt64 {
	return sql.NullInt64{Int64: sysID, Valid: sysID != 0}
}

// Create persists a new demand and assigns its generated ID.
func (r *DemandRepository) Create(ctx context.Context, entity *demand.Demand) error {
	query := `
		INSERT INTO production_demand (
			pd_type, pd_sub_type, pd_source, pd_carry_action, pd_cpm_product_sys_id,
			pd_qty_original, pd_qty_remaining, pd_deadline, pd_customer_id, pd_contract_no,
			pd_contract_date, pd_stuff_advance_no, pd_incoterm, pd_lc_status, pd_grade_requirement,
			pd_ax_min_pct, pd_am_max_pct, pd_carry_from_id, pd_sos_ref, pd_status, pd_month,
			pd_created_by, pd_created_at, pd_updated_at, pd_shade_code, pd_shade_name,
			pd_product_link_reason
		) VALUES (
			$1, NULLIF($2, '')::VARCHAR, $3, NULLIF($4, '')::VARCHAR, $5::BIGINT,
			$6, $7, $8, $9::BIGINT, NULLIF($10, '')::VARCHAR,
			$11::DATE, NULLIF($12, '')::VARCHAR, NULLIF($13, '')::VARCHAR, NULLIF($14, '')::VARCHAR, NULLIF($15, '')::VARCHAR,
			$16::DECIMAL, $17::DECIMAL, $18::BIGINT, $19::BIGINT, $20, $21,
			$22, $23, $24, NULLIF($25, '')::VARCHAR, NULLIF($26, '')::VARCHAR,
			NULLIF($27, '')::VARCHAR
		) RETURNING pd_id`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Type(), entity.SubType(), entity.Source(), entity.CarryAction(), demandProductArg(entity.CpmProductSysID()),
		entity.QtyOriginal(), entity.QtyRemaining(), entity.Deadline(), int64PtrArg(entity.CustomerID()), entity.ContractNo(),
		timePtrArg(entity.ContractDate()), entity.StuffAdvanceNo(), entity.Incoterm(), entity.LcStatus(), entity.GradeReq(),
		floatPtrArg(entity.AxMinPct()), floatPtrArg(entity.AmMaxPct()), int64PtrArg(entity.CarryFromID()), int64PtrArg(entity.SosRef()), entity.Status(), entity.Month(),
		entity.CreatedBy(), entity.CreatedAt(), entity.UpdatedAt(), entity.ShadeCode(), entity.ShadeName(),
		entity.ProductLinkReason(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to create demand: %w", err)
	}
	setDemandID(entity, id)
	return nil
}

// GetByID retrieves a demand by its ID.
func (r *DemandRepository) GetByID(ctx context.Context, id int64) (*demand.Demand, error) {
	query := `SELECT ` + demandColumns + ` FROM production_demand WHERE pd_id = $1`
	return scanDemandRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves demands with filtering and pagination.
func (r *DemandRepository) List(ctx context.Context, filter demand.ListFilter) ([]*demand.Demand, int64, error) {
	filter.Validate()

	base := `FROM production_demand WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if filter.Type != "" {
		base += fmt.Sprintf(` AND pd_type = $%d`, idx)
		args = append(args, filter.Type)
		idx++
	}
	if filter.Status != "" {
		base += fmt.Sprintf(` AND pd_status = $%d`, idx)
		args = append(args, filter.Status)
		idx++
	}
	if filter.Month != "" {
		base += fmt.Sprintf(` AND pd_month = $%d`, idx)
		args = append(args, filter.Month)
		idx++
	}
	if filter.CpmProductSysID != nil {
		base += fmt.Sprintf(` AND pd_cpm_product_sys_id = $%d`, idx)
		args = append(args, *filter.CpmProductSysID)
		idx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND (pd_contract_no ILIKE $%d OR pd_stuff_advance_no ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.WithoutPlan {
		// NOT EXISTS, not a client-side filter: paging would otherwise hide
		// unplanned demands behind planned ones on later pages.
		base += ` AND NOT EXISTS (
			SELECT 1 FROM production_plan_item ppi WHERE ppi.ppi_demand_id = production_demand.pd_id
		)`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count demands: %w", err)
	}

	sortColumnMap := map[string]string{
		"deadline":      "pd_deadline",
		"product_code":  "pd_cpm_product_sys_id",
		"qty_remaining": "pd_qty_remaining",
		"status":        "pd_status",
		"created_at":    "pd_created_at",
	}
	orderCol := "pd_created_at"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + demandColumns + ` ` + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, sortDirection(filter.SortOrder), idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list demands: %w", err)
	}
	defer closeRows(rows)

	result, err := scanDemandRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// Update persists changes to an existing demand.
func (r *DemandRepository) Update(ctx context.Context, entity *demand.Demand) error {
	query := `
		UPDATE production_demand SET
			pd_sub_type = NULLIF($2, '')::VARCHAR, pd_source = $3, pd_carry_action = NULLIF($4, '')::VARCHAR,
			pd_cpm_product_sys_id = $5::BIGINT,
			pd_qty_original = $6, pd_qty_remaining = $7, pd_deadline = $8,
			pd_customer_id = $9::BIGINT, pd_contract_no = NULLIF($10, '')::VARCHAR, pd_contract_date = $11::DATE,
			pd_stuff_advance_no = NULLIF($12, '')::VARCHAR, pd_incoterm = NULLIF($13, '')::VARCHAR,
			pd_lc_status = NULLIF($14, '')::VARCHAR, pd_grade_requirement = $15,
			pd_ax_min_pct = $16::DECIMAL, pd_am_max_pct = $17::DECIMAL,
			pd_status = $18, pd_confirmed_by = $19::BIGINT, pd_confirmed_at = $20::TIMESTAMPTZ,
			pd_updated_at = $21, pd_month = $22,
			pd_shade_code = NULLIF($23, '')::VARCHAR, pd_shade_name = NULLIF($24, '')::VARCHAR,
			pd_product_link_reason = NULLIF($25, '')::VARCHAR
		WHERE pd_id = $1`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(), entity.SubType(), entity.Source(), entity.CarryAction(),
		demandProductArg(entity.CpmProductSysID()),
		entity.QtyOriginal(), entity.QtyRemaining(), entity.Deadline(),
		int64PtrArg(entity.CustomerID()), entity.ContractNo(), timePtrArg(entity.ContractDate()),
		entity.StuffAdvanceNo(), entity.Incoterm(), entity.LcStatus(), entity.GradeReq(),
		floatPtrArg(entity.AxMinPct()), floatPtrArg(entity.AmMaxPct()),
		entity.Status(), int64PtrArg(entity.ConfirmedBy()), timePtrArg(entity.ConfirmedAt()),
		entity.UpdatedAt(), entity.Month(), entity.ShadeCode(), entity.ShadeName(),
		entity.ProductLinkReason(),
	)
	if err != nil {
		return fmt.Errorf("failed to update demand: %w", err)
	}
	return checkAffected(res, demand.ErrNotFound)
}

// Delete removes a demand by its ID.
func (r *DemandRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM production_demand WHERE pd_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete demand: %w", err)
	}
	return checkAffected(res, demand.ErrNotFound)
}

// CountPlanItems returns how many plan items reference the demand.
func (r *DemandRepository) CountPlanItems(ctx context.Context, demandID int64) (int64, error) {
	var n int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM production_plan_item WHERE ppi_demand_id = $1`, demandID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count plan items for demand: %w", err)
	}
	return n, nil
}

// ListCarryCandidates returns demands eligible for carry-forward in a month.
func (r *DemandRepository) ListCarryCandidates(ctx context.Context, sourceMonth string) ([]*demand.Demand, error) {
	query := `SELECT ` + demandColumns + ` FROM production_demand
		WHERE pd_month = $1
		  AND pd_status IN ('PARTIAL', 'IN_PRODUCTION', 'CONFIRMED', 'DEFERRED')
		  AND pd_qty_remaining > 0
		ORDER BY pd_deadline ASC`
	rows, err := r.db.QueryContext(ctx, query, sourceMonth)
	if err != nil {
		return nil, fmt.Errorf("failed to list carry-forward candidates: %w", err)
	}
	defer closeRows(rows)
	return scanDemandRows(rows)
}

func scanDemandRow(row *sql.Row) (*demand.Demand, error) {
	var d demandDTO
	if err := row.Scan(d.dest()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, demand.ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan demand: %w", err)
	}
	return d.toEntity(), nil
}

func scanDemandRows(rows *sql.Rows) ([]*demand.Demand, error) {
	var result []*demand.Demand
	for rows.Next() {
		var d demandDTO
		if err := rows.Scan(d.dest()...); err != nil {
			return nil, fmt.Errorf("failed to scan demand row: %w", err)
		}
		result = append(result, d.toEntity())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating demand rows: %w", err)
	}
	return result, nil
}

type demandDTO struct {
	ID              int64
	Type            string
	SubType         sql.NullString
	Source          string
	CarryAction     sql.NullString
	CpmProductSysID sql.NullInt64
	QtyOriginal     float64
	QtyRemaining    float64
	Deadline        time.Time
	CustomerID      sql.NullInt64
	ContractNo      sql.NullString
	ContractDate    sql.NullTime
	StuffAdvanceNo  sql.NullString
	Incoterm        sql.NullString
	LcStatus        sql.NullString
	GradeReq        sql.NullString
	AxMinPct        sql.NullFloat64
	AmMaxPct        sql.NullFloat64
	CarryFromID     sql.NullInt64
	SosRef          sql.NullInt64
	Status          string
	Month           string
	ConfirmedBy     sql.NullInt64
	ConfirmedAt     sql.NullTime
	CreatedBy       int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ShadeCode       sql.NullString
	ShadeName       sql.NullString
	LinkReason      sql.NullString
}

func (d *demandDTO) dest() []interface{} {
	return []interface{}{
		&d.ID, &d.Type, &d.SubType, &d.Source, &d.CarryAction, &d.CpmProductSysID,
		&d.QtyOriginal, &d.QtyRemaining, &d.Deadline, &d.CustomerID, &d.ContractNo, &d.ContractDate,
		&d.StuffAdvanceNo, &d.Incoterm, &d.LcStatus, &d.GradeReq, &d.AxMinPct, &d.AmMaxPct,
		&d.CarryFromID, &d.SosRef, &d.Status, &d.Month, &d.ConfirmedBy, &d.ConfirmedAt,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ShadeCode, &d.ShadeName,
		&d.LinkReason,
	}
}

func (d *demandDTO) toEntity() *demand.Demand {
	return demand.Reconstruct(demand.ReconstructParams{
		ID:                d.ID,
		Type:              d.Type,
		SubType:           nullString(d.SubType),
		Source:            d.Source,
		CarryAction:       nullString(d.CarryAction),
		CpmProductSysID:   d.CpmProductSysID.Int64,
		QtyOriginal:       d.QtyOriginal,
		QtyRemaining:      d.QtyRemaining,
		Deadline:          d.Deadline,
		CustomerID:        nullInt64Ptr(d.CustomerID),
		ContractNo:        nullString(d.ContractNo),
		ContractDate:      nullTimePtr(d.ContractDate),
		StuffAdvanceNo:    nullString(d.StuffAdvanceNo),
		Incoterm:          nullString(d.Incoterm),
		LcStatus:          nullString(d.LcStatus),
		GradeReq:          nullString(d.GradeReq),
		ShadeCode:         nullString(d.ShadeCode),
		ShadeName:         nullString(d.ShadeName),
		AxMinPct:          nullFloatPtr(d.AxMinPct),
		AmMaxPct:          nullFloatPtr(d.AmMaxPct),
		CarryFromID:       nullInt64Ptr(d.CarryFromID),
		SosRef:            nullInt64Ptr(d.SosRef),
		Status:            d.Status,
		ProductLinkReason: nullString(d.LinkReason),
		Month:             d.Month,
		ConfirmedBy:       nullInt64Ptr(d.ConfirmedBy),
		ConfirmedAt:       nullTimePtr(d.ConfirmedAt),
		CreatedBy:         d.CreatedBy,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
	})
}

// setDemandID re-hydrates the entity with its generated ID by rebuilding it via
// the exported reconstruct path (fields are private).
func setDemandID(entity *demand.Demand, id int64) {
	*entity = *demand.Reconstruct(demand.ReconstructParams{
		ID:                id,
		Type:              entity.Type(),
		SubType:           entity.SubType(),
		Source:            entity.Source(),
		CarryAction:       entity.CarryAction(),
		CpmProductSysID:   entity.CpmProductSysID(),
		QtyOriginal:       entity.QtyOriginal(),
		QtyRemaining:      entity.QtyRemaining(),
		Deadline:          entity.Deadline(),
		CustomerID:        entity.CustomerID(),
		ContractNo:        entity.ContractNo(),
		ContractDate:      entity.ContractDate(),
		StuffAdvanceNo:    entity.StuffAdvanceNo(),
		Incoterm:          entity.Incoterm(),
		LcStatus:          entity.LcStatus(),
		GradeReq:          entity.GradeReq(),
		ShadeCode:         entity.ShadeCode(),
		ShadeName:         entity.ShadeName(),
		AxMinPct:          entity.AxMinPct(),
		AmMaxPct:          entity.AmMaxPct(),
		CarryFromID:       entity.CarryFromID(),
		SosRef:            entity.SosRef(),
		Status:            entity.Status(),
		ProductLinkReason: entity.ProductLinkReason(),
		Month:             entity.Month(),
		ConfirmedBy:       entity.ConfirmedBy(),
		ConfirmedAt:       entity.ConfirmedAt(),
		CreatedBy:         entity.CreatedBy(),
		CreatedAt:         entity.CreatedAt(),
		UpdatedAt:         entity.UpdatedAt(),
	})
}

// ── Sales order staging reads ────────────────────────────────────────────────

const stagingColumns = `sos_id, sos_contract_no, sos_contract_date, sos_contract_sys_id, sos_customer_code,
	sos_customer_name, sos_item_code, sos_item_desc, sos_grade_code, sos_shade_code, sos_shade_name,
	sos_qty_ordered, sos_qty_delivered, sos_qty_remaining, sos_deadline, sos_ship_date, sos_merge_no,
	sos_term, sos_rate, sos_currency, sos_blocked_status, sos_outstanding_ar, sos_pallet_type, sos_end_use,
	sos_mix_flag, sos_annotation, sos_remarks, sos_etl_synced_at, sos_pulled_to_demand_id,
	sos_cpm_product_sys_id, sos_match_status, sos_match_count, sos_matched_at`

// LookupStagingItemCodes returns the Orion item code of each requested staging
// row, keyed by sos id. Rows without an item code are omitted so a caller can
// distinguish "no staging row" from "blank code" by absence alone.
func (r *DemandRepository) LookupStagingItemCodes(ctx context.Context, sosIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(sosIDs))
	if len(sosIDs) == 0 {
		return out, nil
	}
	const query = `
		SELECT sos_id, sos_item_code
		FROM sales_order_staging
		WHERE sos_id = ANY($1) AND COALESCE(sos_item_code, '') <> ''`
	rows, err := r.db.QueryContext(ctx, query, pq.Array(sosIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to look up staging item codes: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			id   int64
			code string
		)
		if err := rows.Scan(&id, &code); err != nil {
			return nil, fmt.Errorf("failed to scan staging item code: %w", err)
		}
		out[id] = code
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating staging item codes: %w", err)
	}
	return out, nil
}

// GetStagingByIDs retrieves SO-staging rows by their ids.
func (r *DemandRepository) GetStagingByIDs(ctx context.Context, ids []int64) ([]*demand.SalesOrderStaging, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `SELECT ` + stagingColumns + ` FROM sales_order_staging WHERE sos_id = ANY($1) ORDER BY sos_id`
	rows, err := r.db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("failed to get staging by ids: %w", err)
	}
	defer closeRows(rows)
	return scanStagingRows(rows)
}

// stagingPredicate builds the shared FROM/WHERE clause for the staging inbox,
// plus its bind args and the next free placeholder index. ListStaging and
// ListStagingIDs both go through it so a "select all matching" can never
// resolve a different set than the count the planner was shown.
func stagingPredicate(filter demand.StagingIDsFilter) (string, []interface{}, int) {
	base := `FROM sales_order_staging WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if filter.UnpulledOnly {
		base += ` AND sos_pulled_to_demand_id IS NULL`
	}
	if filter.CustomerCode != "" {
		base += fmt.Sprintf(` AND sos_customer_code = $%d`, idx)
		args = append(args, filter.CustomerCode)
		idx++
	}
	if filter.ItemCode != "" {
		base += fmt.Sprintf(` AND sos_item_code = $%d`, idx)
		args = append(args, filter.ItemCode)
		idx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND (sos_contract_no ILIKE $%d OR sos_customer_name ILIKE $%d OR sos_item_desc ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	return base, args, idx
}

// ListStagingIDs retrieves the ids of every staging row matching a filter, up
// to limit, along with the untruncated match count. Ordered by deadline so a
// truncated selection is the most urgent slice, not an arbitrary one.
func (r *DemandRepository) ListStagingIDs(ctx context.Context, filter demand.StagingIDsFilter, limit int) ([]int64, int64, error) {
	base, args, idx := stagingPredicate(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count staging rows: %w", err)
	}

	query := `SELECT sos_id ` + base +
		fmt.Sprintf(` ORDER BY sos_deadline ASC, sos_id ASC LIMIT $%d`, idx)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list staging ids: %w", err)
	}
	defer closeRows(rows)

	ids := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan staging id: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate staging ids: %w", err)
	}
	return ids, total, nil
}

// ListStaging retrieves SO-staging rows with filtering and pagination.
func (r *DemandRepository) ListStaging(ctx context.Context, filter demand.StagingListFilter) ([]*demand.SalesOrderStaging, int64, error) {
	filter.Validate()

	base, args, idx := stagingPredicate(filter.Predicate())

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count staging rows: %w", err)
	}

	sortColumnMap := map[string]string{
		"contract_no":   "sos_contract_no",
		"customer_name": "sos_customer_name",
		"deadline":      "sos_deadline",
		"qty_remaining": "sos_qty_remaining",
	}
	orderCol := "sos_deadline"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + stagingColumns + ` ` + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, sortDirection(filter.SortOrder), idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list staging rows: %w", err)
	}
	defer closeRows(rows)

	result, err := scanStagingRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// MarkStagingPulled sets sos_pulled_to_demand_id for a staging row.
func (r *DemandRepository) MarkStagingPulled(ctx context.Context, sosID, demandID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE sales_order_staging SET sos_pulled_to_demand_id = $2 WHERE sos_id = $1`, sosID, demandID)
	if err != nil {
		return fmt.Errorf("failed to mark staging pulled: %w", err)
	}
	return checkAffected(res, demand.ErrNotFound)
}

// SetStagingProduct writes a planner's manual product pick onto a staging row
// and marks it MANUAL so ApplyStagingResolutions (which skips MANUAL rows)
// never overwrites it on the next ETL pass. Rows already pulled into a demand
// are rejected: their product is owned by the demand from that point on.
func (r *DemandRepository) SetStagingProduct(ctx context.Context, sosID, cpmProductSysID int64) (*demand.SalesOrderStaging, error) {
	query := `
		UPDATE sales_order_staging
		SET sos_cpm_product_sys_id = $2,
		    sos_match_status       = $3,
		    sos_match_count        = 1,
		    sos_matched_at         = NOW()
		WHERE sos_id = $1
		  AND sos_pulled_to_demand_id IS NULL
		RETURNING ` + stagingColumns

	rows, err := r.db.QueryContext(ctx, query, sosID, cpmProductSysID, demand.MatchStatusManual)
	if err != nil {
		return nil, fmt.Errorf("failed to set staging product: %w", err)
	}
	defer closeRows(rows)

	updated, err := scanStagingRows(rows)
	if err != nil {
		return nil, err
	}
	if len(updated) == 0 {
		return nil, demand.ErrStagingNotUpdatable
	}
	return updated[0], nil
}

// stagingPairKeyExpr normalizes a staging row's (item, shade) key the same way
// finance normalizes cost_product_master: trimmed, upper-cased, NULL as empty.
const stagingPairKeyExpr = `UPPER(TRIM(COALESCE(sos_item_code, ''))), UPPER(TRIM(COALESCE(sos_shade_code, '')))`

// ListUnresolvedStagingPairs returns the distinct normalized (item, shade) keys
// of not-yet-pulled staging rows awaiting product resolution.
func (r *DemandRepository) ListUnresolvedStagingPairs(ctx context.Context) ([]demand.StagingPair, error) {
	query := `
		SELECT DISTINCT ` + stagingPairKeyExpr + `
		FROM sales_order_staging
		WHERE sos_pulled_to_demand_id IS NULL
		  AND sos_match_status = $1
		  AND COALESCE(sos_item_code, '') <> ''
		ORDER BY 1, 2`
	rows, err := r.db.QueryContext(ctx, query, demand.MatchStatusUnresolved)
	if err != nil {
		return nil, fmt.Errorf("failed to list unresolved staging pairs: %w", err)
	}
	defer closeRows(rows)

	var pairs []demand.StagingPair
	for rows.Next() {
		var p demand.StagingPair
		if err := rows.Scan(&p.ItemCode, &p.ShadeCode); err != nil {
			return nil, fmt.Errorf("failed to scan unresolved staging pair: %w", err)
		}
		pairs = append(pairs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating unresolved staging pairs: %w", err)
	}
	return pairs, nil
}

// ApplyStagingResolutions writes each resolution onto every not-yet-pulled
// staging row sharing its normalized pair. Rows already marked MANUAL keep the
// planner's pick. The whole batch is one transaction so a partial write cannot
// leave the inbox half-resolved.
func (r *DemandRepository) ApplyStagingResolutions(ctx context.Context, resolutions []demand.ProductResolution) (int64, error) {
	if len(resolutions) == 0 {
		return 0, nil
	}
	items, shades, sysIDs, statuses, counts := stagingResolutionArrays(resolutions)

	query := `
		UPDATE sales_order_staging AS s
		SET sos_cpm_product_sys_id = u.sys_id,
		    sos_match_status       = u.status,
		    sos_match_count        = u.match_count,
		    sos_matched_at         = NOW()
		FROM (
			SELECT UPPER(TRIM(i)) AS item, UPPER(TRIM(sh)) AS shade, sid AS sys_id, st AS status, mc AS match_count
			FROM UNNEST($1::text[], $2::text[], $3::bigint[], $4::text[], $5::int[]) AS t(i, sh, sid, st, mc)
		) AS u
		WHERE s.sos_pulled_to_demand_id IS NULL
		  AND s.sos_match_status <> $6
		  AND UPPER(TRIM(COALESCE(s.sos_item_code, ''))) = u.item
		  AND UPPER(TRIM(COALESCE(s.sos_shade_code, ''))) = u.shade`

	res, err := r.db.ExecContext(ctx, query,
		pq.Array(items), pq.Array(shades), pq.Array(sysIDs), pq.Array(statuses), pq.Array(counts),
		demand.MatchStatusManual,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to apply staging resolutions: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read staging resolution row count: %w", err)
	}
	return affected, nil
}

// stagingResolutionArrays pivots resolutions into the parallel arrays the
// UNNEST-driven bulk update consumes.
func stagingResolutionArrays(resolutions []demand.ProductResolution) (items, shades []string, sysIDs []sql.NullInt64, statuses []string, counts []int32) {
	n := len(resolutions)
	items = make([]string, 0, n)
	shades = make([]string, 0, n)
	sysIDs = make([]sql.NullInt64, 0, n)
	statuses = make([]string, 0, n)
	counts = make([]int32, 0, n)
	for _, res := range resolutions {
		items = append(items, res.Pair.ItemCode)
		shades = append(shades, res.Pair.ShadeCode)
		sysID := sql.NullInt64{}
		if res.CpmProductSysID != nil {
			sysID = sql.NullInt64{Int64: *res.CpmProductSysID, Valid: true}
		}
		sysIDs = append(sysIDs, sysID)
		statuses = append(statuses, res.MatchStatus())
		counts = append(counts, res.MatchCount)
	}
	return items, shades, sysIDs, statuses, counts
}

func scanStagingRows(rows *sql.Rows) ([]*demand.SalesOrderStaging, error) {
	var result []*demand.SalesOrderStaging
	for rows.Next() {
		var s stagingDTO
		if err := rows.Scan(s.dest()...); err != nil {
			return nil, fmt.Errorf("failed to scan staging row: %w", err)
		}
		result = append(result, s.toModel())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating staging rows: %w", err)
	}
	return result, nil
}

type stagingDTO struct {
	SosID            int64
	ContractNo       sql.NullString
	ContractDate     sql.NullTime
	ContractSysID    sql.NullInt64
	CustomerCode     sql.NullString
	CustomerName     sql.NullString
	ItemCode         sql.NullString
	ItemDesc         sql.NullString
	GradeCode        sql.NullString
	ShadeCode        sql.NullString
	ShadeName        sql.NullString
	QtyOrdered       sql.NullFloat64
	QtyDelivered     sql.NullFloat64
	QtyRemaining     sql.NullFloat64
	Deadline         sql.NullTime
	ShipDate         sql.NullString
	MergeNo          sql.NullString
	Term             sql.NullString
	Rate             sql.NullFloat64
	Currency         sql.NullString
	BlockedStatus    sql.NullString
	OutstandingAr    sql.NullFloat64
	PalletType       sql.NullString
	EndUse           sql.NullString
	MixFlag          sql.NullString
	Annotation       sql.NullString
	Remarks          sql.NullString
	EtlSyncedAt      sql.NullTime
	PulledToDemandID sql.NullInt64
	CpmProductSysID  sql.NullInt64
	MatchStatus      sql.NullString
	MatchCount       sql.NullInt32
	MatchedAt        sql.NullTime
}

func (s *stagingDTO) dest() []interface{} {
	return []interface{}{
		&s.SosID, &s.ContractNo, &s.ContractDate, &s.ContractSysID, &s.CustomerCode,
		&s.CustomerName, &s.ItemCode, &s.ItemDesc, &s.GradeCode, &s.ShadeCode, &s.ShadeName,
		&s.QtyOrdered, &s.QtyDelivered, &s.QtyRemaining, &s.Deadline, &s.ShipDate, &s.MergeNo,
		&s.Term, &s.Rate, &s.Currency, &s.BlockedStatus, &s.OutstandingAr, &s.PalletType, &s.EndUse,
		&s.MixFlag, &s.Annotation, &s.Remarks, &s.EtlSyncedAt, &s.PulledToDemandID,
		&s.CpmProductSysID, &s.MatchStatus, &s.MatchCount, &s.MatchedAt,
	}
}

func (s *stagingDTO) toModel() *demand.SalesOrderStaging {
	return &demand.SalesOrderStaging{
		SosID:            s.SosID,
		ContractNo:       nullString(s.ContractNo),
		ContractDate:     nullTimePtr(s.ContractDate),
		ContractSysID:    nullInt64Ptr(s.ContractSysID),
		CustomerCode:     nullString(s.CustomerCode),
		CustomerName:     nullString(s.CustomerName),
		ItemCode:         nullString(s.ItemCode),
		ItemDesc:         nullString(s.ItemDesc),
		GradeCode:        nullString(s.GradeCode),
		ShadeCode:        nullString(s.ShadeCode),
		ShadeName:        nullString(s.ShadeName),
		QtyOrdered:       s.QtyOrdered.Float64,
		QtyDelivered:     s.QtyDelivered.Float64,
		QtyRemaining:     s.QtyRemaining.Float64,
		Deadline:         nullTimePtr(s.Deadline),
		ShipDate:         nullString(s.ShipDate),
		MergeNo:          nullString(s.MergeNo),
		Term:             nullString(s.Term),
		Rate:             s.Rate.Float64,
		Currency:         nullString(s.Currency),
		BlockedStatus:    nullString(s.BlockedStatus),
		OutstandingAr:    s.OutstandingAr.Float64,
		PalletType:       nullString(s.PalletType),
		EndUse:           nullString(s.EndUse),
		MixFlag:          nullString(s.MixFlag),
		Annotation:       nullString(s.Annotation),
		Remarks:          nullString(s.Remarks),
		EtlSyncedAt:      nullTimePtr(s.EtlSyncedAt),
		PulledToDemandID: nullInt64Ptr(s.PulledToDemandID),
		CpmProductSysID:  nullInt64Ptr(s.CpmProductSysID),
		MatchStatus:      nullString(s.MatchStatus),
		MatchCount:       s.MatchCount.Int32,
		MatchedAt:        nullTimePtr(s.MatchedAt),
	}
}
