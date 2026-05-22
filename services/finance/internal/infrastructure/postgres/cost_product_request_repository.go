package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

// CostProductRequestRepository implements costproductrequest.Repository.
type CostProductRequestRepository struct{ db *DB }

// NewCostProductRequestRepository constructs the repo.
func NewCostProductRequestRepository(db *DB) *CostProductRequestRepository {
	return &CostProductRequestRepository{db: db}
}

var _ costproductrequest.Repository = (*CostProductRequestRepository)(nil)

const cprCols = `
	cpr_request_id,cpr_request_no,cpr_request_type_id,cpr_title,cpr_description,
	cpr_customer_name,cpr_customer_code,cpr_product_classification,cpr_verified_classification,
	cpr_classification_override_reason,cpr_target_volume,cpr_target_price_range,
	cpr_urgency_level,cpr_needed_by_date,cpr_status,cpr_closed_substatus,
	cpr_feasibility_decision,cpr_feasibility_note,cpr_feasibility_by,cpr_feasibility_at,
	cpr_reject_reason,cpr_cancel_reason,cpr_assigned_to_user_id,cpr_requester_user_id,
	cpr_created_at,cpr_updated_at,
	COALESCE(cpr_existing_product_sys_id, 0),
	COALESCE(cpr_linked_route_head_id, 0)`

const cpsCols = `
	cps_spec_id,cps_request_id,cps_raw_material_type,cps_product_description,
	cps_shade_id,cps_shade_custom_text,cps_paper_tube_type_id,
	cps_weight_per_bobbin_kg,cps_box_type,cps_created_at,cps_created_by`

// Create inserts request + (optional) spec in one tx. Generates request_no via SQL function.
func (r *CostProductRequestRepository) Create(ctx context.Context, req *costproductrequest.Request) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const qReq = `
		INSERT INTO cost_product_request (
			cpr_request_no,cpr_request_type_id,cpr_title,cpr_description,
			cpr_customer_name,cpr_customer_code,cpr_product_classification,
			cpr_target_volume,cpr_target_price_range,cpr_urgency_level,
			cpr_needed_by_date,cpr_status,cpr_requester_user_id,
			cpr_linked_route_head_id,
			cpr_created_at,cpr_updated_at
		) VALUES (
			generate_cost_request_no($13),
			$1, $2, NULLIF($3,''), $4, NULLIF($5,''), $6,
			NULLIF($7,'')::numeric, NULLIF($8,''), $9,
			NULLIF($10,'')::date, $11, $12,
			NULLIF($14, 0)::bigint,
			$13, $13
		)
		RETURNING cpr_request_id,cpr_request_no`
	var requestID int64
	var requestNo string
	if err := tx.QueryRowContext(ctx, qReq,
		req.RequestTypeID(), req.Title(), req.Description(),
		req.CustomerName(), req.CustomerCode(), req.ProductClassification(),
		req.TargetVolume(), req.TargetPriceRange(), req.UrgencyLevel(),
		req.NeededByDate(), req.Status(), req.RequesterUserID(),
		req.CreatedAt(),
		req.LinkedRouteHeadID(),
	).Scan(&requestID, &requestNo); err != nil {
		if isCprUniqueViolation(err) {
			return costproductrequest.ErrAlreadyExists
		}
		return fmt.Errorf("insert cost_product_request: %w", err)
	}
	req.SetIDs(requestID, requestNo)

	if s := req.Spec(); s != nil {
		specID, sErr := insertSpec(ctx, tx, requestID, s)
		if sErr != nil {
			return sErr
		}
		req.SetSpecID(specID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Save mutates the request + replaces the spec row (delete + insert) inside one tx.
func (r *CostProductRequestRepository) Save(ctx context.Context, req *costproductrequest.Request) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const qUpd = `
		UPDATE cost_product_request SET
			cpr_request_type_id=$2, cpr_title=$3, cpr_description=NULLIF($4,''),
			cpr_customer_name=$5, cpr_customer_code=NULLIF($6,''),
			cpr_product_classification=$7, cpr_verified_classification=NULLIF($8,''),
			cpr_classification_override_reason=NULLIF($9,''),
			cpr_target_volume=NULLIF($10,'')::numeric, cpr_target_price_range=NULLIF($11,''),
			cpr_urgency_level=$12, cpr_needed_by_date=NULLIF($13,'')::date,
			cpr_status=$14, cpr_closed_substatus=NULLIF($15,''),
			cpr_feasibility_decision=NULLIF($16,''), cpr_feasibility_note=NULLIF($17,''),
			cpr_feasibility_by=NULLIF($18,''), cpr_feasibility_at=$19,
			cpr_reject_reason=NULLIF($20,''), cpr_cancel_reason=NULLIF($21,''),
			cpr_assigned_to_user_id=NULLIF($22,''),
			cpr_existing_product_sys_id=NULLIF($23, 0)::bigint,
			cpr_linked_route_head_id=NULLIF($24, 0)::bigint,
			cpr_updated_at=$25
		WHERE cpr_request_id=$1`
	res, err := tx.ExecContext(ctx, qUpd,
		req.RequestID(),
		req.RequestTypeID(), req.Title(), req.Description(),
		req.CustomerName(), req.CustomerCode(),
		req.ProductClassification(), req.VerifiedClassification(),
		req.ClassificationOverrideReason(),
		req.TargetVolume(), req.TargetPriceRange(),
		req.UrgencyLevel(), req.NeededByDate(),
		req.Status(), req.ClosedSubstatus(),
		req.FeasibilityDecision(), req.FeasibilityNote(),
		req.FeasibilityBy(), timePtrToNullTime(req.FeasibilityAt()),
		req.RejectReason(), req.CancelReason(),
		req.AssignedToUserID(),
		req.ExistingProductSysID(),
		req.LinkedRouteHeadID(),
		req.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("update cost_product_request: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costproductrequest.ErrNotFound
	}

	// Replace spec atomically.
	if _, err := tx.ExecContext(ctx, `DELETE FROM cost_product_spec WHERE cps_request_id=$1`, req.RequestID()); err != nil {
		return fmt.Errorf("delete spec: %w", err)
	}
	if s := req.Spec(); s != nil {
		specID, sErr := insertSpec(ctx, tx, req.RequestID(), s)
		if sErr != nil {
			return sErr
		}
		req.SetSpecID(specID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit save: %w", err)
	}
	return nil
}

// GetByID loads request + spec.
func (r *CostProductRequestRepository) GetByID(ctx context.Context, id int64) (*costproductrequest.Request, error) {
	return r.loadOne(ctx, `cpr_request_id=$1`, id)
}

// GetByNo loads request + spec.
func (r *CostProductRequestRepository) GetByNo(ctx context.Context, requestNo string) (*costproductrequest.Request, error) {
	return r.loadOne(ctx, `cpr_request_no=$1`, requestNo)
}

func (r *CostProductRequestRepository) loadOne(ctx context.Context, predicate string, arg any) (*costproductrequest.Request, error) {
	q := `SELECT ` + cprCols + ` FROM cost_product_request WHERE ` + predicate
	row := r.db.QueryRowContext(ctx, q, arg)
	in, err := scanCprRow(row)
	if err != nil {
		return nil, err
	}
	spec, err := r.loadSpec(ctx, in.RequestID)
	if err != nil {
		return nil, err
	}
	in.Spec = spec
	return costproductrequest.Reconstruct(in), nil
}

func (r *CostProductRequestRepository) loadSpec(ctx context.Context, requestID int64) (*costproductrequest.Spec, error) {
	q := `SELECT ` + cpsCols + ` FROM cost_product_spec WHERE cps_request_id=$1`
	row := r.db.QueryRowContext(ctx, q, requestID)
	s, err := scanCpsRow(row)
	if err != nil {
		if errors.Is(err, costproductrequest.ErrNotFound) {
			return nil, nil // no spec is expected for classification=existing
		}
		return nil, err
	}
	return s, nil
}

// List returns a filtered paginated list (without specs — keep payload light).
func (r *CostProductRequestRepository) List(ctx context.Context, f costproductrequest.Filter) ([]*costproductrequest.Request, int64, error) {
	where := "FROM cost_product_request WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(cpr_request_no) LIKE LOWER($%d) OR LOWER(cpr_title) LIKE LOWER($%d) OR LOWER(cpr_customer_name) LIKE LOWER($%d))`, idx, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.Status != "" {
		where += fmt.Sprintf(` AND cpr_status=$%d`, idx)
		args = append(args, f.Status)
		idx++
	}
	if f.RequestTypeID > 0 {
		where += fmt.Sprintf(` AND cpr_request_type_id=$%d`, idx)
		args = append(args, f.RequestTypeID)
		idx++
	}
	if f.RequesterUserID != "" {
		where += fmt.Sprintf(` AND cpr_requester_user_id=$%d`, idx)
		args = append(args, f.RequesterUserID)
		idx++
	}
	if f.AssigneeUserID != "" {
		where += fmt.Sprintf(` AND cpr_assigned_to_user_id=$%d`, idx)
		args = append(args, f.AssigneeUserID)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_product_request: %w", err)
	}

	sortCol := `cpr_created_at`
	switch f.SortBy {
	case "request_no":
		sortCol = `cpr_request_no`
	case "updated_at":
		sortCol = `cpr_updated_at`
	case "status":
		sortCol = `cpr_status`
	}
	dir := "DESC"
	if strings.EqualFold(f.SortOrder, "asc") {
		dir = "ASC"
	}
	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + cprCols + ` ` + where + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, sortCol, dir, idx, idx+1)
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_product_request: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []*costproductrequest.Request{}
	for rows.Next() {
		in, sErr := scanCprRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		out = append(out, costproductrequest.Reconstruct(in))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_product_request: %w", err)
	}
	return out, total, nil
}

// =============================================================================
// helpers
// =============================================================================

func insertSpec(ctx context.Context, tx *sql.Tx, requestID int64, s *costproductrequest.Spec) (int64, error) {
	const q = `
		INSERT INTO cost_product_spec (
			cps_request_id,cps_raw_material_type,cps_product_description,
			cps_shade_id,cps_shade_custom_text,cps_paper_tube_type_id,
			cps_weight_per_bobbin_kg,cps_box_type,cps_created_at,cps_created_by
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7::numeric,$8,$9,$10)
		RETURNING cps_spec_id`
	var specID int64
	var shadeID sql.NullInt32
	if s.ShadeID != nil {
		shadeID = sql.NullInt32{Int32: *s.ShadeID, Valid: true}
	}
	if err := tx.QueryRowContext(ctx, q,
		requestID, s.RawMaterialType, s.ProductDescription,
		shadeID, s.ShadeCustomText, s.PaperTubeTypeID,
		s.WeightPerBobbinKg, s.BoxType, s.CreatedAt, s.CreatedBy,
	).Scan(&specID); err != nil {
		return 0, fmt.Errorf("insert cost_product_spec: %w", err)
	}
	return specID, nil
}

// =============================================================================
// scanners
// =============================================================================

func scanCprRow(row *sql.Row) (costproductrequest.ReconstructInput, error) {
	in, err := scanCpr(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return costproductrequest.ReconstructInput{}, costproductrequest.ErrNotFound
	}
	return in, err
}

func scanCprRows(rows *sql.Rows) (costproductrequest.ReconstructInput, error) {
	return scanCpr(rows.Scan)
}

func scanCpr(scan func(...any) error) (costproductrequest.ReconstructInput, error) {
	var (
		requestID                                                         int64
		requestNo                                                         string
		requestTypeID                                                     int32
		title                                                             string
		description, customerCode, verifiedClass, overrideReason          sql.NullString
		customerName, productClass                                        string
		targetVolume                                                      sql.NullString
		targetPrice                                                       sql.NullString
		urgency, status                                                   string
		neededByDate                                                      sql.NullTime
		closedSub, feasDecision, feasNote, feasBy, rejectReason, cancelReason, assignee sql.NullString
		feasAt                                                            sql.NullTime
		requester                                                         string
		createdAt, updatedAt                                              time.Time
		existingProductSysID                                              int64
		linkedRouteHeadID                                                 int64
	)
	if err := scan(
		&requestID, &requestNo, &requestTypeID, &title, &description,
		&customerName, &customerCode, &productClass, &verifiedClass,
		&overrideReason, &targetVolume, &targetPrice,
		&urgency, &neededByDate, &status, &closedSub,
		&feasDecision, &feasNote, &feasBy, &feasAt,
		&rejectReason, &cancelReason, &assignee, &requester,
		&createdAt, &updatedAt,
		&existingProductSysID,
		&linkedRouteHeadID,
	); err != nil {
		return costproductrequest.ReconstructInput{}, err
	}
	in := costproductrequest.ReconstructInput{
		RequestID:                    requestID,
		RequestNo:                    requestNo,
		RequestTypeID:                requestTypeID,
		Title:                        title,
		Description:                  description.String,
		CustomerName:                 customerName,
		CustomerCode:                 customerCode.String,
		ProductClassification:        productClass,
		VerifiedClassification:       verifiedClass.String,
		ClassificationOverrideReason: overrideReason.String,
		TargetVolume:                 targetVolume.String,
		TargetPriceRange:             targetPrice.String,
		UrgencyLevel:                 urgency,
		Status:                       status,
		ClosedSubstatus:              closedSub.String,
		FeasibilityDecision:          feasDecision.String,
		FeasibilityNote:              feasNote.String,
		FeasibilityBy:                feasBy.String,
		RejectReason:                 rejectReason.String,
		CancelReason:                 cancelReason.String,
		AssignedToUserID:             assignee.String,
		RequesterUserID:              requester,
		ExistingProductSysID:         existingProductSysID,
		LinkedRouteHeadID:            linkedRouteHeadID,
		CreatedAt:                    createdAt,
		UpdatedAt:                    updatedAt,
	}
	if neededByDate.Valid {
		in.NeededByDate = neededByDate.Time.Format("2006-01-02")
	}
	if feasAt.Valid {
		t := feasAt.Time
		in.FeasibilityAt = &t
	}
	return in, nil
}

func scanCpsRow(row *sql.Row) (*costproductrequest.Spec, error) {
	var (
		specID, requestID                  int64
		rawMat, desc                       string
		shadeID                            sql.NullInt32
		shadeCustom                        sql.NullString
		paperTubeID                        int32
		weight                             string
		boxType                            string
		createdAt                          time.Time
		createdBy                          string
	)
	if err := row.Scan(&specID, &requestID, &rawMat, &desc, &shadeID, &shadeCustom, &paperTubeID, &weight, &boxType, &createdAt, &createdBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costproductrequest.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_product_spec: %w", err)
	}
	s := &costproductrequest.Spec{
		SpecID:             specID,
		RawMaterialType:    rawMat,
		ProductDescription: desc,
		ShadeCustomText:    shadeCustom.String,
		PaperTubeTypeID:    paperTubeID,
		WeightPerBobbinKg:  weight,
		BoxType:            boxType,
		CreatedAt:          createdAt,
		CreatedBy:          createdBy,
	}
	if shadeID.Valid {
		v := shadeID.Int32
		s.ShadeID = &v
	}
	return s, nil
}

func timePtrToNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func isCprUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
