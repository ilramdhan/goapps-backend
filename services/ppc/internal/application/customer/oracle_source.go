// Package customer provides application-layer use cases for the PPC customer master.
package customer

import (
	"context"

	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
)

// OracleReader is the narrow read port the customer sync needs from Oracle.
// Declared here (not in the oracle package) so the domain never depends on a
// driver type, mirroring how machinesync declares OracleMachineSource.
type OracleReader interface {
	ListCustomers(ctx context.Context) ([]oracle.CustomerRow, error)
}

// oracleSource adapts raw OM_CUSTOMER rows to the domain Source port.
type oracleSource struct {
	reader OracleReader
}

// NewOracleSource wraps an Oracle reader as a domain customer source. A nil
// reader yields nil so the caller's sync degrades to a no-op instead of panicking
// on a typed-nil interface.
func NewOracleSource(reader OracleReader) customerdomain.Source {
	if reader == nil {
		return nil
	}
	return &oracleSource{reader: reader}
}

// ListCustomers reads OM_CUSTOMER and maps each row onto the domain transport
// struct. CUST_FRZ_FLAG_NUM is inverted here: Orion records "frozen", PPC records
// "active", and doing the flip once at the boundary keeps the rest honest.
func (s *oracleSource) ListCustomers(ctx context.Context) ([]customerdomain.Sourced, error) {
	rows, err := s.reader.ListCustomers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]customerdomain.Sourced, 0, len(rows))
	for _, row := range rows {
		result = append(result, customerdomain.Sourced{
			Code:            row.Code,
			Name:            row.Name,
			ShortName:       optionalText(row.ShortName),
			TaxNo:           optionalText(row.TaxNo),
			ParentCode:      optionalText(row.ParentCode),
			IsActive:        !row.Frozen,
			SourceCreatedAt: row.CreatedAt,
			SourceUpdatedAt: row.UpdatedAt,
		})
	}
	return result, nil
}

// optionalText maps an empty Oracle string onto nil so the column stores NULL.
func optionalText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
