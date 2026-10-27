package dashboard_test

import (
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/dashboard"
)

func TestParseKpiConfig_CrossRatioTypes(t *testing.T) {
	cfg, err := dashboard.ParseKpiConfig([]map[string]any{{
		"label": "Sick Rate", "agg": "cross_ratio", "scale": float64(100),
		"cross_ratio_numerator_type": "HR_SICK_DAYS", "cross_ratio_denominator_type": "HR_SCHED_DAYS",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg[0].CrossRatioNumeratorType != "HR_SICK_DAYS" || cfg[0].CrossRatioDenominatorType != "HR_SCHED_DAYS" {
		t.Errorf("types not parsed: %+v", cfg[0])
	}
}
