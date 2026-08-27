package grpc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/boxbobbincost"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/intermingling"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// --- test doubles -----------------------------------------------------------

// stubParamRepo implements parameter.Repository but only GetByFillGroup is
// exercised by the fill handlers; the rest panic so an accidental call is loud.
type stubParamRepo struct {
	parameter.Repository
	children []*parameter.Parameter
}

func (s *stubParamRepo) GetByFillGroup(_ context.Context, _ string) ([]*parameter.Parameter, error) {
	return s.children, nil
}

type stubInterminglingRepo struct {
	intermingling.Repository
	entity *intermingling.Entity
}

func (s *stubInterminglingRepo) GetByCode(_ context.Context, _ string) (*intermingling.Entity, error) {
	return s.entity, nil
}

type stubBoxBobbinRepo struct {
	boxbobbincost.Repository
	entity *boxbobbincost.Entity
	rates  []*boxbobbincost.RateEntry
}

func (s *stubBoxBobbinRepo) GetByCode(_ context.Context, _ string) (*boxbobbincost.Entity, error) {
	return s.entity, nil
}

func (s *stubBoxBobbinRepo) ListRates(_ context.Context, _ uuid.UUID) ([]*boxbobbincost.RateEntry, error) {
	return s.rates, nil
}

// --- helpers ----------------------------------------------------------------

// newFillParam builds a child param whose lookup_source_column is srcCol.
func newFillParam(t *testing.T, code, srcCol string) *parameter.Parameter {
	t.Helper()
	c, err := parameter.NewCode(code)
	require.NoError(t, err)
	p, err := parameter.NewParameter(
		c, code, "", parameter.DataTypeNumber, parameter.ParamCategoryInput,
		nil, nil, nil, nil,
		parameter.CostingMetadata{LookupSourceColumn: srcCol},
		"admin",
	)
	require.NoError(t, err)
	return p
}

// captureWarnings runs fn with a zerolog logger bound to the context and returns
// the decoded log records it emitted.
func captureWarnings(t *testing.T, fn func(ctx context.Context)) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.WarnLevel)
	fn(logger.WithContext(context.Background()))

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		out = append(out, rec)
	}
	return out
}

// --- tests ------------------------------------------------------------------

// TestFillFromIntermingling_WarnsOnUnknownColumn covers the numeric-only side of
// R30: intermingling has no text reader map, so any column missing from
// interminglingNumericReaders is unfillable and must be logged, not swallowed.
func TestFillFromIntermingling_WarnsOnUnknownColumn(t *testing.T) {
	intm, err := intermingling.New("INTM01", "Intermingling 01", 1.25, "", "admin")
	require.NoError(t, err)

	h := &YarnLookupFillHandler{
		interminglingRepo: &stubInterminglingRepo{entity: intm},
		paramRepo: &stubParamRepo{children: []*parameter.Parameter{
			newFillParam(t, "INTM_COST", "intm_cost_per_kg"),
			newFillParam(t, "INTM_GHOST", "intm_column_that_does_not_exist"),
		}},
	}

	var resp any
	recs := captureWarnings(t, func(ctx context.Context) {
		r, ferr := h.fillFromIntermingling(ctx, "INTM01", "TRIGGER_PARAM")
		require.NoError(t, ferr)
		resp = r
	})
	require.NotNil(t, resp)

	require.Len(t, recs, 1, "exactly the unknown column should warn")
	assert.Equal(t, "intm_column_that_does_not_exist", recs[0]["lookup_source_column"])
	assert.Equal(t, "INTM_GHOST", recs[0]["param_code"])
	assert.Equal(t, "TRIGGER_PARAM", recs[0]["source_param_code"])
}

// TestFillFromBoxBobbinCost_WarnsOnlyOnUnknownColumn pins the switch-based
// variant: the default case must warn, but a known column that deliberately
// skips filling because its rate is zero must NOT warn.
func TestFillFromBoxBobbinCost_WarnsOnlyOnUnknownColumn(t *testing.T) {
	bbc, err := boxbobbincost.New("BBC01", "Box 01", "BOX", 24, "",
		nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	// Rates present but zero → bbcr_* cases hit the "> 0" guard and skip silently.
	rates := []*boxbobbincost.RateEntry{
		boxbobbincost.NewRateEntry(bbc.ID(), "202601", 0, 0, nil, nil, "admin"),
	}

	h := &YarnLookupFillHandler{
		boxBobbinRepo: &stubBoxBobbinRepo{entity: bbc, rates: rates},
		paramRepo: &stubParamRepo{children: []*parameter.Parameter{
			newFillParam(t, "BBC_NOB", "no_of_bob"),
			newFillParam(t, "BBC_BOB_RATE", "bbcr_bob_rate_mkt"),
			newFillParam(t, "BBC_BOX_RATE", "bbcr_box_rate_mkt"),
			newFillParam(t, "BBC_GHOST", "bbc_column_that_does_not_exist"),
		}},
	}

	var recs []map[string]any
	var numFills map[string]float64
	recs = captureWarnings(t, func(ctx context.Context) {
		r, ferr := h.fillFromBoxBobbinCost(ctx, "BBC01", "TRIGGER_PARAM")
		require.NoError(t, ferr)
		numFills = r.GetNumericFills()
	})

	// Zero rates are a deliberate no-fill, not a missing reader.
	assert.Equal(t, map[string]float64{"BBC_NOB": 24}, numFills)

	require.Len(t, recs, 1, "only the unknown column should warn, not the zero-rate skips")
	assert.Equal(t, "bbc_column_that_does_not_exist", recs[0]["lookup_source_column"])
	assert.Equal(t, "BBC_GHOST", recs[0]["param_code"])
	assert.Equal(t, "TRIGGER_PARAM", recs[0]["source_param_code"])
}
