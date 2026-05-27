//go:build stress

// Package stress provides shared infrastructure for the S8e.2 calc-engine
// stress scenarios. The harness is intentionally lightweight: scenarios run
// against a populated stress corpus (see ./fixtures/seed_calc_corpus.go) and
// emit JSON reports under ./reports/.
//
// Activation: every test in this package requires STRESS_TEST=true and the
// `stress` build tag. Otherwise scenarios are skipped so normal `go test ./...`
// runs don't pull in this code.
package stress

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// ScenarioMetrics is the canonical shape emitted by each scenario.
type ScenarioMetrics struct {
	Scenario     string  `json:"scenario"`
	Timestamp    string  `json:"timestamp"`
	ProductCount int     `json:"product_count"`
	ChunkCount   int     `json:"chunk_count"`
	P50ChunkMS   float64 `json:"p50_chunk_ms"`
	P95ChunkMS   float64 `json:"p95_chunk_ms"`
	P99ChunkMS   float64 `json:"p99_chunk_ms"`
	TotalMS      float64 `json:"total_ms"`
	SuccessCount int     `json:"success_count"`
	BlockedCount int     `json:"blocked_count"`
	FailedCount  int     `json:"failed_count"`
	Notes        string  `json:"notes,omitempty"`
}

// Baseline mirrors a single entry inside baseline.json.
type Baseline struct {
	P50ChunkMS float64 `json:"p50_chunk_ms"`
	P95ChunkMS float64 `json:"p95_chunk_ms"`
	TotalMS    float64 `json:"total_ms"`
}

// requireStress short-circuits any scenario when STRESS_TEST != "true".
func requireStress(t *testing.T) {
	t.Helper()
	if os.Getenv("STRESS_TEST") != "true" {
		t.Skip("STRESS_TEST=true not set — skipping stress scenario")
	}
}

// percentile returns the p-th percentile (0..100) of a slice of durations in
// milliseconds. The input is sorted in place.
func percentile(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	sort.Float64s(samples)
	if p <= 0 {
		return samples[0]
	}
	if p >= 100 {
		return samples[len(samples)-1]
	}
	idx := int(float64(len(samples)-1) * p / 100)
	return samples[idx]
}

// writeReport persists ScenarioMetrics to tests/stress/reports/<scenario>_<ts>.json.
func writeReport(t *testing.T, m ScenarioMetrics) {
	t.Helper()
	if err := os.MkdirAll("reports", 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	path := filepath.Join("reports", fmt.Sprintf("%s_%s.json", m.Scenario, stamp))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		t.Fatalf("encode report: %v", err)
	}
	t.Logf("wrote report: %s", path)
}

// loadBaseline reads tests/stress/baseline.json and returns the entry for the
// given scenario key (e.g. "s4_full_12k"). Returns nil if not found / unreadable
// so the caller can decide to skip the regression assertion on first run.
func loadBaseline(t *testing.T, key string) *Baseline {
	t.Helper()
	raw, err := os.ReadFile("baseline.json")
	if err != nil {
		t.Logf("baseline.json not found (%v) — skipping regression assertion", err)
		return nil
	}
	var all map[string]Baseline
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Logf("baseline.json malformed: %v", err)
		return nil
	}
	b, ok := all[key]
	if !ok {
		return nil
	}
	return &b
}

// assertNoP95Regression fails the test if observed p95 exceeds baseline by
// more than 20%. Skips silently when baseline is nil (first-run case).
func assertNoP95Regression(t *testing.T, scenarioKey string, observed float64) {
	t.Helper()
	b := loadBaseline(t, scenarioKey)
	if b == nil || b.P95ChunkMS == 0 {
		return
	}
	limit := b.P95ChunkMS * 1.20
	if observed > limit {
		t.Fatalf("regression: p95 %.1fms > 120%% of baseline %.1fms (limit %.1fms)",
			observed, b.P95ChunkMS, limit)
	}
}
