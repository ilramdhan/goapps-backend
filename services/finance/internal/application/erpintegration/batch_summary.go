package erpintegration

// batch_summary.go merges a step summary into ceib_summary.<step> (design
// §10: result_summary is also merged into the batch summary).

import (
	"encoding/json"
	"fmt"
)

// mergeStepSummary returns summary with summary[step] = v, keeping the other
// steps' entries. A corrupt or non-object summary is replaced.
func mergeStepSummary(summary []byte, step string, v any) ([]byte, error) {
	m := map[string]json.RawMessage{}
	if len(summary) > 0 {
		if err := json.Unmarshal(summary, &m); err != nil || m == nil {
			m = map[string]json.RawMessage{}
		}
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal %s summary: %w", step, err)
	}
	m[step] = raw
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal batch summary: %w", err)
	}
	return out, nil
}
