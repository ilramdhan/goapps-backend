// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	pmp "github.com/mutugading/goapps-backend/services/ppc/internal/domain/productmachineparameter"
)

// ProductMachineParameterRepository implements pmp.Repository using PostgreSQL.
type ProductMachineParameterRepository struct {
	db *DB
}

// NewProductMachineParameterRepository creates a new ProductMachineParameterRepository.
func NewProductMachineParameterRepository(db *DB) *ProductMachineParameterRepository {
	return &ProductMachineParameterRepository{db: db}
}

var _ pmp.Repository = (*ProductMachineParameterRepository)(nil)

const pmpSelectColumns = `pmp.pmp_id, pmp.pmp_cpm_product_sys_id, pmp.pmp_machine_id, m.machine_no, ` +
	`pmp.pmp_param_id, pmp.pmp_value_num, pmp.pmp_value_text, pmp.pmp_value_flag, pmp.pmp_updated_at`

// Create persists a new parameter value and assigns its generated ID.
func (r *ProductMachineParameterRepository) Create(ctx context.Context, entity *pmp.Parameter) error {
	query := `
		INSERT INTO product_machine_parameter (
			pmp_cpm_product_sys_id, pmp_machine_id, pmp_param_id,
			pmp_value_num, pmp_value_text, pmp_value_flag
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING pmp_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.CpmProductSysID(),
		entity.MachineID(),
		entity.ParamID(),
		floatPtrToNull(entity.ValueNum()),
		stringPtrToNull(entity.ValueText()),
		boolPtrToNull(entity.ValueFlag()),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return pmp.ErrAlreadyExists
		}
		if isForeignKeyViolation(err) {
			return pmp.ErrInvalidMachine
		}
		return fmt.Errorf("failed to create product machine parameter: %w", err)
	}

	*entity = *pmp.Reconstruct(
		id, entity.CpmProductSysID(), entity.MachineID(), entity.MachineNo(),
		entity.ParamID(), entity.ValueNum(), entity.ValueText(), entity.ValueFlag(),
		entity.UpdatedAt(),
	)
	return nil
}

// GetByID retrieves a parameter value by its ID.
func (r *ProductMachineParameterRepository) GetByID(ctx context.Context, id int64) (*pmp.Parameter, error) {
	query := `SELECT ` + pmpSelectColumns + `
		FROM product_machine_parameter pmp
		LEFT JOIN machine m ON pmp.pmp_machine_id = m.machine_id
		WHERE pmp.pmp_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves parameter values with filtering and pagination.
func (r *ProductMachineParameterRepository) List(ctx context.Context, filter pmp.ListFilter) ([]*pmp.Parameter, int64, error) {
	filter.Validate()

	base := `FROM product_machine_parameter pmp
		LEFT JOIN machine m ON pmp.pmp_machine_id = m.machine_id
		WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.CpmProductSysID != nil {
		base += fmt.Sprintf(` AND pmp.pmp_cpm_product_sys_id = $%d`, argIdx)
		args = append(args, *filter.CpmProductSysID)
		argIdx++
	}
	if filter.MachineID != nil {
		base += fmt.Sprintf(` AND pmp.pmp_machine_id = $%d`, argIdx)
		args = append(args, *filter.MachineID)
		argIdx++
	}
	if filter.ParamID != "" {
		base += fmt.Sprintf(` AND pmp.pmp_param_id = $%d`, argIdx)
		args = append(args, filter.ParamID)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count product machine parameters: %w", err)
	}

	sortColumnMap := map[string]string{
		"cpm_product_sys_id": "pmp.pmp_cpm_product_sys_id",
		"machine_id":         "pmp.pmp_machine_id",
		"param_id":           "pmp.pmp_param_id",
		"updated_at":         "pmp.pmp_updated_at",
	}
	orderCol := "pmp.pmp_cpm_product_sys_id"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + pmpSelectColumns + ` ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list product machine parameters: %w", err)
	}
	defer closeRows(rows)

	var result []*pmp.Parameter
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating product machine parameter rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing parameter value.
func (r *ProductMachineParameterRepository) Update(ctx context.Context, entity *pmp.Parameter) error {
	query := `
		UPDATE product_machine_parameter
		SET pmp_value_num = $2, pmp_value_text = $3, pmp_value_flag = $4, pmp_updated_at = $5
		WHERE pmp_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		floatPtrToNull(entity.ValueNum()),
		stringPtrToNull(entity.ValueText()),
		boolPtrToNull(entity.ValueFlag()),
		timePtrArg(entity.UpdatedAt()),
	)
	if err != nil {
		return fmt.Errorf("failed to update product machine parameter: %w", err)
	}
	return checkAffected(res, pmp.ErrNotFound)
}

// Delete removes a parameter value by its ID.
func (r *ProductMachineParameterRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM product_machine_parameter WHERE pmp_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete product machine parameter: %w", err)
	}
	return checkAffected(res, pmp.ErrNotFound)
}

func (r *ProductMachineParameterRepository) scanRow(row *sql.Row) (*pmp.Parameter, error) {
	var dto pmpDTO
	err := dto.scan(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, pmp.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan product machine parameter: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *ProductMachineParameterRepository) scanRows(rows *sql.Rows) (*pmp.Parameter, error) {
	var dto pmpDTO
	if err := dto.scan(rows.Scan); err != nil {
		return nil, fmt.Errorf("failed to scan product machine parameter row: %w", err)
	}
	return dto.toEntity(), nil
}

// pmpDTO is the row-mapping type for product_machine_parameter.
type pmpDTO struct {
	ID              int64
	CpmProductSysID int64
	MachineID       int64
	MachineNo       sql.NullString
	ParamID         string
	ValueNum        sql.NullFloat64
	ValueText       sql.NullString
	ValueFlag       sql.NullBool
	UpdatedAt       sql.NullTime
}

func (d *pmpDTO) scan(scan func(...any) error) error {
	return scan(
		&d.ID, &d.CpmProductSysID, &d.MachineID, &d.MachineNo,
		&d.ParamID, &d.ValueNum, &d.ValueText, &d.ValueFlag, &d.UpdatedAt,
	)
}

func (d *pmpDTO) toEntity() *pmp.Parameter {
	return pmp.Reconstruct(
		d.ID,
		d.CpmProductSysID,
		d.MachineID,
		nullString(d.MachineNo),
		d.ParamID,
		nullFloatPtr(d.ValueNum),
		nullStringPtr(d.ValueText),
		nullBoolPtr(d.ValueFlag),
		nullTimePtr(d.UpdatedAt),
	)
}
