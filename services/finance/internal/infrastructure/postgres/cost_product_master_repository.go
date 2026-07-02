package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// CostProductMasterRepository implements costproductmaster.Repository.
type CostProductMasterRepository struct{ db *DB }

// NewCostProductMasterRepository constructs the repository.
func NewCostProductMasterRepository(db *DB) *CostProductMasterRepository {
	return &CostProductMasterRepository{db: db}
}

var _ costproductmaster.Repository = (*CostProductMasterRepository)(nil)

const cpmColumns = `
	cpm_product_sys_id,cpm_product_code,cpm_product_type_id,cpm_product_name,
	cpm_shade_code,cpm_grade_code,cpm_description,
	cpm_erp_item_code,cpm_erp_grade_code_1,cpm_erp_grade_code_2,
	cpm_erp_linked_at,cpm_erp_linked_by,
	cpm_is_active,
	cpm_created_at,cpm_created_by,cpm_updated_at,cpm_updated_by,
	COALESCE(cpm_shade_name,''),COALESCE(cpm_flex_01,''),COALESCE(cpm_flex_02,''),COALESCE(cpm_flex_03,'')`

// Create inserts the product. product_code is generated atomically via generate_cost_product_code()
// inside the same INSERT, returning the new sys_id and code.
func (r *CostProductMasterRepository) Create(ctx context.Context, p *costproductmaster.CostProductMaster) error {
	const q = `
		INSERT INTO cost_product_master (
			cpm_product_code,cpm_product_type_id,cpm_product_name,cpm_shade_code,cpm_grade_code,cpm_description,
			cpm_is_active,cpm_created_at,cpm_created_by,cpm_updated_at,cpm_updated_by
		)
		VALUES (
			generate_cost_product_code($1, $7), $1, $2, $3, $4, $5,
			$6, $7, $8, $7, $8
		)
		RETURNING cpm_product_sys_id,cpm_product_code`
	var sysID int64
	var code string
	if err := r.db.QueryRowContext(ctx, q,
		p.ProductTypeID(), p.ProductName(), p.ShadeCode(), p.GradeCode(), p.Description(),
		p.IsActive(), p.CreatedAt(), p.CreatedBy(),
	).Scan(&sysID, &code); err != nil {
		if isProductMasterUniqueViolation(err) {
			return costproductmaster.ErrAlreadyExists
		}
		return fmt.Errorf("create cost_product_master: %w", err)
	}
	p.SetGeneratedCode(sysID, code)
	return nil
}

// GetBySysID loads by sys_id.
func (r *CostProductMasterRepository) GetBySysID(ctx context.Context, sysID int64) (*costproductmaster.CostProductMaster, error) {
	q := `SELECT ` + cpmColumns + ` FROM cost_product_master WHERE cpm_product_sys_id=$1`
	return r.scanRow(r.db.QueryRowContext(ctx, q, sysID))
}

// GetByCode loads by product_code.
func (r *CostProductMasterRepository) GetByCode(ctx context.Context, code string) (*costproductmaster.CostProductMaster, error) {
	q := `SELECT ` + cpmColumns + ` FROM cost_product_master WHERE cpm_product_code=$1`
	return r.scanRow(r.db.QueryRowContext(ctx, q, code))
}

// Update saves descriptive fields + legacy flex fields + ERP linkage + active flag.
func (r *CostProductMasterRepository) Update(ctx context.Context, p *costproductmaster.CostProductMaster) error {
	const q = `
		UPDATE cost_product_master SET
			cpm_product_name=$2,cpm_shade_code=$3,cpm_grade_code=$4,cpm_description=$5,
			cpm_flex_01=$6,cpm_flex_02=$7,cpm_flex_03=$8,
			cpm_erp_item_code=$9,cpm_erp_grade_code_1=$10,cpm_erp_grade_code_2=$11,
			cpm_erp_linked_at=$12,cpm_erp_linked_by=$13,
			cpm_is_active=$14,cpm_updated_at=$15,cpm_updated_by=$16
		WHERE cpm_product_sys_id=$1`
	var erpItem, erpGrade1, erpGrade2, erpBy sql.NullString
	if p.ErpItemCode() != "" {
		erpItem = sql.NullString{String: p.ErpItemCode(), Valid: true}
	}
	if p.ErpGradeCode1() != "" {
		erpGrade1 = sql.NullString{String: p.ErpGradeCode1(), Valid: true}
	}
	if p.ErpGradeCode2() != "" {
		erpGrade2 = sql.NullString{String: p.ErpGradeCode2(), Valid: true}
	}
	if p.ErpLinkedBy() != "" {
		erpBy = sql.NullString{String: p.ErpLinkedBy(), Valid: true}
	}
	var erpAt sql.NullTime
	if p.ErpLinkedAt() != nil {
		erpAt = sql.NullTime{Time: *p.ErpLinkedAt(), Valid: true}
	}
	res, err := r.db.ExecContext(ctx, q,
		p.ProductSysID(), p.ProductName(), p.ShadeCode(), p.GradeCode(), p.Description(),
		p.Flex01(), p.Flex02(), p.Flex03(),
		erpItem, erpGrade1, erpGrade2, erpAt, erpBy,
		p.IsActive(), p.UpdatedAt(), p.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update cost_product_master: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costproductmaster.ErrNotFound
	}
	return nil
}

// cpmSortColumn maps an API sort key to its ORDER BY expression. Every value
// comes from this fixed map only — never from user input — so the returned
// expression is safe to interpolate into the query.
func cpmSortColumn(sortBy string) string {
	switch sortBy {
	case "product_name":
		return "cpm_product_name"
	case sortKeyCreatedAt:
		return "cpm_created_at"
	case "updated_at":
		return "cpm_updated_at"
	case "product_type_code":
		return "(SELECT cpt_type_code FROM cost_product_type WHERE cpt_type_id = cpm_product_type_id)"
	case "shade_code":
		return "cpm_shade_code"
	case "grade_code":
		return "cpm_grade_code"
	case "oracle_sys_id":
		return "cpm_flex_02"
	case "erp_compound_key":
		return "cpm_flex_01"
	case "type_label":
		return "cpm_flex_03"
	case sortKeyStatus:
		return "cpm_is_active"
	default:
		return cpmSortColProductCode
	}
}

const cpmSortColProductCode = "cpm_product_code"

// cpmOrderBy builds the ORDER BY body for the given sort key + direction,
// appending a stable secondary ordering on product_code whenever the primary
// sort column is not product_code itself.
func cpmOrderBy(sortBy, sortOrder string) string {
	col := cpmSortColumn(sortBy)
	dir := sortASC
	if strings.EqualFold(sortOrder, "desc") {
		dir = sortDESC
	}
	orderBy := col + " " + dir
	if col != cpmSortColProductCode {
		orderBy += ", " + cpmSortColProductCode + " " + sortASC
	}
	return orderBy
}

// cpmEffectiveTypeIDs returns the deduplicated union of the legacy single
// ProductTypeID (when > 0) and the ProductTypeIDs slice (ignoring non-positive
// entries), as int64 for driver-friendly array binding.
func cpmEffectiveTypeIDs(f costproductmaster.Filter) []int64 {
	ids := make([]int64, 0, len(f.ProductTypeIDs)+1)
	seen := make(map[int32]bool, len(f.ProductTypeIDs)+1)
	if f.ProductTypeID > 0 {
		seen[f.ProductTypeID] = true
		ids = append(ids, int64(f.ProductTypeID))
	}
	for _, id := range f.ProductTypeIDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, int64(id))
		}
	}
	return ids
}

// List returns a filtered paginated list.
func (r *CostProductMasterRepository) List(ctx context.Context, f costproductmaster.Filter) ([]*costproductmaster.CostProductMaster, int64, error) { //nolint:gocognit // filter + sort + pagination builder
	where := "FROM cost_product_master WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(cpm_product_code) LIKE LOWER($%d) OR LOWER(cpm_product_name) LIKE LOWER($%d) OR LOWER(COALESCE(cpm_erp_item_code,'')) LIKE LOWER($%d) OR LOWER(COALESCE(cpm_flex_02,'')) LIKE LOWER($%d))`, idx, idx, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	typeIDs := cpmEffectiveTypeIDs(f)
	switch {
	case len(typeIDs) == 1:
		where += fmt.Sprintf(` AND cpm_product_type_id=$%d`, idx)
		args = append(args, typeIDs[0])
		idx++
	case len(typeIDs) > 1:
		where += fmt.Sprintf(` AND cpm_product_type_id = ANY($%d)`, idx)
		args = append(args, pq.Array(typeIDs))
		idx++
	}
	if f.ShadeCode != "" {
		where += fmt.Sprintf(` AND cpm_shade_code=$%d`, idx)
		args = append(args, f.ShadeCode)
		idx++
	}
	switch f.ActiveFilter {
	case filterActive:
		where += ` AND cpm_is_active=TRUE`
	case filterInactive:
		where += ` AND cpm_is_active=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_product_master: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + cpmColumns + ` ` + where + fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, cpmOrderBy(f.SortBy, f.SortOrder), idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_product_master: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	items := []*costproductmaster.CostProductMaster{}
	for rows.Next() {
		p, sErr := r.scanRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_product_master: %w", err)
	}
	return items, total, nil
}

// BulkCreate upserts a batch of products in a single transaction.
// Returns product_code → assigned sysID mapping for FK resolution by callers.
func (r *CostProductMasterRepository) BulkCreate(ctx context.Context, items []*costproductmaster.CostProductMaster, updatedBy string) (map[string]int64, error) {
	if len(items) == 0 {
		return map[string]int64{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin BulkCreate tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	// $1=code $2=typeID $3=name $4=shadeCode $5=gradeCode $6=desc
	// $7=shadeName $8=flex01 $9=flex02 $10=flex03
	// $11=isActive $12=now $13=updatedBy
	const q = `
		INSERT INTO cost_product_master (
			cpm_product_code,cpm_product_type_id,cpm_product_name,
			cpm_shade_code,cpm_grade_code,cpm_description,
			cpm_shade_name,cpm_flex_01,cpm_flex_02,cpm_flex_03,
			cpm_is_active,cpm_created_at,cpm_created_by,cpm_updated_at,cpm_updated_by
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $12, $13
		)
		ON CONFLICT (cpm_product_code) DO UPDATE SET
			cpm_product_name    = EXCLUDED.cpm_product_name,
			cpm_shade_code      = EXCLUDED.cpm_shade_code,
			cpm_grade_code      = EXCLUDED.cpm_grade_code,
			cpm_description     = EXCLUDED.cpm_description,
			cpm_shade_name      = EXCLUDED.cpm_shade_name,
			cpm_flex_01         = EXCLUDED.cpm_flex_01,
			cpm_flex_02         = EXCLUDED.cpm_flex_02,
			cpm_flex_03         = EXCLUDED.cpm_flex_03,
			cpm_updated_at      = EXCLUDED.cpm_updated_at,
			cpm_updated_by      = EXCLUDED.cpm_updated_by
		RETURNING cpm_product_code, cpm_product_sys_id`

	result := make(map[string]int64, len(items))
	now := time.Now().UTC()
	for _, p := range items {
		var code string
		var sysID int64
		if scanErr := tx.QueryRowContext(ctx, q,
			p.ProductCode(), p.ProductTypeID(), p.ProductName(),
			p.ShadeCode(), p.GradeCode(), p.Description(),
			p.ShadeName(), p.Flex01(), p.Flex02(), p.Flex03(),
			p.IsActive(), now, updatedBy,
		).Scan(&code, &sysID); scanErr != nil {
			return nil, fmt.Errorf("BulkCreate upsert: %w", scanErr)
		}
		result[code] = sysID
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit BulkCreate: %w", err)
	}
	return result, nil
}

const upsertByLegacyBatchSize = 200

// BulkUpsertByLegacyID upserts products in batches of 200.
// legacy_sys_id (cpm_flex_02) is tried first as a conflict key; if absent, cpm_product_code
// is used. The result maps each input LegacySysID (which may be an Oracle integer string or
// a product_code when the product has no Oracle link) to its assigned cpm_product_sys_id.
func (r *CostProductMasterRepository) BulkUpsertByLegacyID(ctx context.Context, items []costproductmaster.ProductUpsertInput, actor string) ([]costproductmaster.ProductUpsertResult, error) {
	if len(items) == 0 {
		return []costproductmaster.ProductUpsertResult{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin BulkUpsertByLegacyID tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	const q = `
		INSERT INTO cost_product_master (
			cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_grade_code, cpm_description,
			cpm_shade_name, cpm_flex_01, cpm_flex_02, cpm_flex_03,
			cpm_erp_item_code, cpm_is_active,
			cpm_created_at, cpm_created_by, cpm_updated_at, cpm_updated_by,
			cpm_product_code
		)
		VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7, $8, $9,
			$10, $11,
			$12, $13, $12, $13,
			COALESCE(
				(SELECT cpm_product_code FROM cost_product_master WHERE cpm_flex_02 = $14 AND cpm_flex_02 <> '' AND cpm_is_active = TRUE),
				(SELECT cpm_product_code FROM cost_product_master WHERE cpm_product_code = $14 AND cpm_is_active = TRUE),
				generate_cost_product_code($1, $12)
			)
		)
		ON CONFLICT (cpm_product_code)
		DO UPDATE SET
			cpm_product_type_id = EXCLUDED.cpm_product_type_id,
			cpm_product_name    = EXCLUDED.cpm_product_name,
			cpm_shade_code      = EXCLUDED.cpm_shade_code,
			cpm_grade_code      = EXCLUDED.cpm_grade_code,
			cpm_description     = EXCLUDED.cpm_description,
			cpm_shade_name      = EXCLUDED.cpm_shade_name,
			cpm_flex_01         = EXCLUDED.cpm_flex_01,
			cpm_flex_02         = EXCLUDED.cpm_flex_02,
			cpm_flex_03         = EXCLUDED.cpm_flex_03,
			cpm_erp_item_code   = EXCLUDED.cpm_erp_item_code,
			cpm_is_active       = EXCLUDED.cpm_is_active,
			cpm_updated_at      = EXCLUDED.cpm_updated_at,
			cpm_updated_by      = EXCLUDED.cpm_updated_by
		RETURNING cpm_product_sys_id, xmax::text`

	results := make([]costproductmaster.ProductUpsertResult, 0, len(items))
	now := time.Now().UTC()

	for start := 0; start < len(items); start += upsertByLegacyBatchSize {
		end := min(start+upsertByLegacyBatchSize, len(items))
		batch := items[start:end]
		batchResults, err := r.upsertLegacyBatch(ctx, tx, q, batch, actor, now)
		if err != nil {
			return nil, err
		}
		results = append(results, batchResults...)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit BulkUpsertByLegacyID: %w", err)
	}
	return results, nil
}

func (r *CostProductMasterRepository) upsertLegacyBatch(
	ctx context.Context,
	tx *sql.Tx,
	q string,
	batch []costproductmaster.ProductUpsertInput,
	actor string,
	now time.Time,
) ([]costproductmaster.ProductUpsertResult, error) {
	results := make([]costproductmaster.ProductUpsertResult, 0, len(batch))
	for _, item := range batch {
		var sysID int64
		var xmax string
		if err := tx.QueryRowContext(ctx, q,
			item.ProductTypeID, item.ProductName,
			item.ShadeCode, item.GradeCode, item.Description,
			item.ShadeName, item.Flex01, item.LegacySysID, item.Flex03,
			item.ErpItemCode, item.IsActive,
			now, actor,
			item.LegacySysID, // $14 — separate copy avoids SQLSTATE 42P08 type-inference conflict
		).Scan(&sysID, &xmax); err != nil {
			return nil, fmt.Errorf("BulkUpsertByLegacyID upsert row: %w", err)
		}
		// Use the input LegacySysID as the map key, not cpm_flex_02 from RETURNING.
		// When a product has no Oracle link, flex02 is empty but the export wrote
		// product_code as legacy_oracle_sys_id — we need that same value as the key
		// so downstream sheets (CPP, CAPP, route_head, etc.) can look it up.
		results = append(results, costproductmaster.ProductUpsertResult{
			LegacySysID:  item.LegacySysID,
			ProductSysID: sysID,
			WasInserted:  xmax == "0",
		})
	}
	return results, nil
}

// ListAll returns all products matching the filter with no pagination cap.
func (r *CostProductMasterRepository) ListAll(ctx context.Context, f costproductmaster.Filter) ([]*costproductmaster.CostProductMaster, error) {
	f.Page = 1
	f.PageSize = 100000
	items, _, err := r.List(ctx, f)
	return items, err
}

// =============================================================================
// scan helpers
// =============================================================================

// RollbackImport deletes all data written by a failed bulk import for the given
// newly-inserted product IDs. Runs in a single transaction; no-op if the slice is empty.
// Order: cost_route_rm → cost_route_seq → cost_route_head → cost_product_master
// (cost_product_parameter + cost_product_applicable_param are cleaned via ON DELETE CASCADE).
func (r *CostProductMasterRepository) RollbackImport(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("RollbackImport begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	steps := []struct {
		table  string
		column string
	}{
		{"cost_route_rm", "crm_parent_product_sys_id"},
		{"cost_route_seq", "crs_product_sys_id"},
		{"cost_route_head", "crh_product_sys_id"},
		{"cost_product_master", "cpm_product_sys_id"},
	}
	for _, s := range steps {
		q := fmt.Sprintf(`DELETE FROM %s WHERE %s = ANY($1)`, s.table, s.column) //nolint:gosec // table/column names are hardcoded
		if _, delErr := tx.ExecContext(ctx, q, pq.Array(ids)); delErr != nil {
			return fmt.Errorf("RollbackImport delete %s: %w", s.table, delErr)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("RollbackImport commit: %w", err)
	}
	return nil
}

type cpmRow struct {
	sysID        int64
	code         string
	typeID       int32
	name         string
	shade, grade sql.NullString
	desc         sql.NullString
	erpItem      sql.NullString
	erpG1, erpG2 sql.NullString
	erpAt        sql.NullTime
	erpBy        sql.NullString
	active       bool
	createdAt    time.Time
	createdBy    string
	updatedAt    time.Time
	updatedBy    string
	shadeName    string
	flex01       string
	flex02       string
	flex03       string
}

func (r *CostProductMasterRepository) scanRow(row *sql.Row) (*costproductmaster.CostProductMaster, error) {
	var d cpmRow
	if err := row.Scan(
		&d.sysID, &d.code, &d.typeID, &d.name,
		&d.shade, &d.grade, &d.desc,
		&d.erpItem, &d.erpG1, &d.erpG2,
		&d.erpAt, &d.erpBy,
		&d.active,
		&d.createdAt, &d.createdBy, &d.updatedAt, &d.updatedBy,
		&d.shadeName, &d.flex01, &d.flex02, &d.flex03,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costproductmaster.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_product_master: %w", err)
	}
	return cpmFromRow(d), nil
}

func (r *CostProductMasterRepository) scanRows(rows *sql.Rows) (*costproductmaster.CostProductMaster, error) {
	var d cpmRow
	if err := rows.Scan(
		&d.sysID, &d.code, &d.typeID, &d.name,
		&d.shade, &d.grade, &d.desc,
		&d.erpItem, &d.erpG1, &d.erpG2,
		&d.erpAt, &d.erpBy,
		&d.active,
		&d.createdAt, &d.createdBy, &d.updatedAt, &d.updatedBy,
		&d.shadeName, &d.flex01, &d.flex02, &d.flex03,
	); err != nil {
		return nil, fmt.Errorf("scan cost_product_master row: %w", err)
	}
	return cpmFromRow(d), nil
}

func cpmFromRow(d cpmRow) *costproductmaster.CostProductMaster {
	var erpAt *time.Time
	if d.erpAt.Valid {
		t := d.erpAt.Time
		erpAt = &t
	}
	grade := d.grade.String
	if grade == "" {
		grade = "AX"
	}
	return costproductmaster.Reconstruct(
		d.sysID, d.code, d.typeID,
		d.name, d.shade.String, grade, d.desc.String,
		d.erpItem.String, d.erpG1.String, d.erpG2.String,
		erpAt, d.erpBy.String,
		d.active,
		d.createdAt, d.createdBy, d.updatedAt, d.updatedBy,
		d.shadeName, d.flex01, d.flex02, d.flex03,
	)
}

func isProductMasterUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}

// ListAllLegacyIDs returns a map of flex02OrCode → cpm_product_sys_id for all
// active products. flex02OrCode = cpm_flex_02 if set, else cpm_product_code.
// Used by the params-only import to resolve legacy_oracle_sys_id without
// requiring a product_master sheet in the same file.
func (r *CostProductMasterRepository) ListAllLegacyIDs(ctx context.Context) (map[string]int64, error) {
	const q = `SELECT cpm_product_sys_id, cpm_product_code, COALESCE(cpm_flex_02, '')
               FROM cost_product_master
               WHERE cpm_is_active = TRUE`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list product legacy IDs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	result := make(map[string]int64)
	for rows.Next() {
		var sysID int64
		var code, flex02 string
		if scanErr := rows.Scan(&sysID, &code, &flex02); scanErr != nil {
			return nil, fmt.Errorf("scan product legacy ID: %w", scanErr)
		}
		key := flex02
		if key == "" {
			key = code
		}
		result[key] = sysID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product legacy IDs: %w", err)
	}
	return result, nil
}
