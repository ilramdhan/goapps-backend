package postgres

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The T-CUR repository is read-only by contract (P0-T10a): every statement
// must be a single SELECT with no DML/DDL keyword anywhere in its text.
func TestCurrencySanityQueries_AreSelectOnly(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE|TRUNCATE|DROP|ALTER|CREATE|GRANT|REVOKE)\b|;`)
	for name, q := range map[string]string{
		"labels":      currencyLabelDistributionQuery,
		"percentiles": currencyPercentilesQuery,
		"count":       currencyOutlierCountQuery,
		"list":        currencyOutlierListQuery,
		"periods":     currencyPeriodSummaryQuery,
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, strings.HasPrefix(strings.TrimSpace(q), "SELECT"), "must start with SELECT")
			assert.Empty(t, forbidden.FindAllString(q, -1))
			assert.Contains(t, q, "cpc_calculation_type = 'ACTUAL'")
			assert.Contains(t, q, "cpc_status <> 'SUPERSEDED'")
		})
	}
}
