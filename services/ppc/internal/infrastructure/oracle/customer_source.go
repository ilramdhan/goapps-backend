package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// CustomerRow is a raw row from Oracle MGTDAT.OM_CUSTOMER used by the customer sync.
// Only the columns PPC plans against are read: OM_CUSTOMER carries addresses, bank
// details, credit terms and remarks that belong to Orion, not to production planning.
type CustomerRow struct {
	// Code is OM_CUSTOMER.CUST_CODE, the natural key.
	Code string
	// Name is OM_CUSTOMER.CUST_NAME.
	Name string
	// ShortName is OM_CUSTOMER.CUST_SHORT_NAME; empty when the source is null.
	ShortName string
	// TaxNo is OM_CUSTOMER.CUST_TAX_REGN_NO (NPWP).
	TaxNo string
	// ParentCode is OM_CUSTOMER.CUST_PARENT_CODE (group head), empty when standalone.
	ParentCode string
	// Frozen mirrors OM_CUSTOMER.CUST_FRZ_FLAG_NUM: 1 means frozen in Orion.
	Frozen bool
	// CreatedAt is OM_CUSTOMER.CUST_CR_DT.
	CreatedAt *time.Time
	// UpdatedAt is OM_CUSTOMER.CUST_UPD_DT.
	UpdatedAt *time.Time
}

// customerQuery is the read-only projection of OM_CUSTOMER the PPC sync consumes.
// Schema-qualified: the ETL user's default schema is not guaranteed to be MGTDAT,
// and an unqualified name would resolve against the caller's own schema.
const customerQuery = `
	SELECT CUST_CODE,
	       CUST_NAME,
	       CUST_SHORT_NAME,
	       CUST_TAX_REGN_NO,
	       CUST_PARENT_CODE,
	       CUST_FRZ_FLAG_NUM,
	       CUST_CR_DT,
	       CUST_UPD_DT
	FROM MGTDAT.OM_CUSTOMER`

// ListCustomers reads the Orion customer master. Read-only (SELECT only).
// A nil client (Oracle unconfigured or unreachable at startup) yields no rows and
// no error, so the sync degrades to a no-op rather than failing the service.
func (c *Client) ListCustomers(ctx context.Context) ([]CustomerRow, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, customerQuery)
	if err != nil {
		return nil, fmt.Errorf("query OM_CUSTOMER: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close OM_CUSTOMER rows")
		}
	}()

	var result []CustomerRow
	for rows.Next() {
		row, scanErr := scanCustomerRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate OM_CUSTOMER rows: %w", err)
	}
	return result, nil
}

// scanCustomerRow reads one OM_CUSTOMER row defensively: every column is scanned
// through a nullable type because the Orion master carries plenty of nulls.
func scanCustomerRow(rows *sql.Rows) (CustomerRow, error) {
	var (
		code, name, shortName, taxNo, parentCode sql.NullString
		frozen                                   sql.NullInt64
		createdAt, updatedAt                     sql.NullTime
	)
	if err := rows.Scan(&code, &name, &shortName, &taxNo, &parentCode, &frozen, &createdAt, &updatedAt); err != nil {
		return CustomerRow{}, fmt.Errorf("scan OM_CUSTOMER row: %w", err)
	}
	return CustomerRow{
		Code:       strings.TrimSpace(code.String),
		Name:       strings.TrimSpace(name.String),
		ShortName:  strings.TrimSpace(shortName.String),
		TaxNo:      strings.TrimSpace(taxNo.String),
		ParentCode: strings.TrimSpace(parentCode.String),
		Frozen:     frozen.Valid && frozen.Int64 == 1,
		CreatedAt:  optionalTime(createdAt),
		UpdatedAt:  optionalTime(updatedAt),
	}, nil
}

// optionalTime maps a nullable Oracle DATE onto an optional timestamp.
func optionalTime(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}
