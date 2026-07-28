package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// ProductMachineValueSource resolves per-(product,machine) parameter values from
// the product_machine_parameter table (resolution layer 2).
type ProductMachineValueSource struct {
	db *DB
}

// NewProductMachineValueSource builds the layer-2 parameter value source.
func NewProductMachineValueSource(db *DB) *ProductMachineValueSource {
	return &ProductMachineValueSource{db: db}
}

var _ workorder.ProductMachineValueSource = (*ProductMachineValueSource)(nil)

// ProductMachineValues returns typed values keyed by param id for a product+machine.
func (s *ProductMachineValueSource) ProductMachineValues(ctx context.Context, productSysID, machineID int64) (map[string]workorder.TypedValue, error) {
	query := `SELECT pmp_param_id::TEXT, pmp_value_num, pmp_value_text, pmp_value_flag
		FROM product_machine_parameter
		WHERE pmp_cpm_product_sys_id = $1 AND pmp_machine_id = $2`
	rows, err := s.db.QueryContext(ctx, query, productSysID, machineID)
	if err != nil {
		return nil, fmt.Errorf("failed to load product-machine parameters: %w", err)
	}
	defer closeRows(rows)

	values := make(map[string]workorder.TypedValue)
	for rows.Next() {
		var paramID string
		var num sql.NullFloat64
		var text sql.NullString
		var flag sql.NullBool
		if err := rows.Scan(&paramID, &num, &text, &flag); err != nil {
			return nil, fmt.Errorf("failed to scan product-machine parameter: %w", err)
		}
		values[paramID] = workorder.TypedValue{
			Num:  nullFloatPtr(num),
			Text: nullStringPtr(text),
			Flag: nullBoolPtr(flag),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating product-machine parameters: %w", err)
	}
	return values, nil
}

// WORefValueSource resolves a referenced WO's PPC parameter values from the
// wo_parameter table (resolution layer 1).
type WORefValueSource struct {
	db *DB
}

// NewWORefValueSource builds the layer-1 (WO reference) parameter value source.
func NewWORefValueSource(db *DB) *WORefValueSource {
	return &WORefValueSource{db: db}
}

var _ workorder.WORefValueSource = (*WORefValueSource)(nil)

// RefParamValues returns the referenced WO's PPC values keyed by param id.
func (s *WORefValueSource) RefParamValues(ctx context.Context, refWoID int64) (map[string]workorder.TypedValue, error) {
	query := `SELECT wop_param_id::TEXT, wop_value_ppc_num, wop_value_ppc_text, wop_value_ppc_flag
		FROM wo_parameter WHERE wop_wo_id = $1`
	rows, err := s.db.QueryContext(ctx, query, refWoID)
	if err != nil {
		return nil, fmt.Errorf("failed to load reference wo parameters: %w", err)
	}
	defer closeRows(rows)

	values := make(map[string]workorder.TypedValue)
	for rows.Next() {
		var paramID string
		var num sql.NullFloat64
		var text sql.NullString
		var flag sql.NullBool
		if err := rows.Scan(&paramID, &num, &text, &flag); err != nil {
			return nil, fmt.Errorf("failed to scan reference wo parameter: %w", err)
		}
		values[paramID] = workorder.TypedValue{
			Num:  nullFloatPtr(num),
			Text: nullStringPtr(text),
			Flag: nullBoolPtr(flag),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reference wo parameters: %w", err)
	}
	return values, nil
}
