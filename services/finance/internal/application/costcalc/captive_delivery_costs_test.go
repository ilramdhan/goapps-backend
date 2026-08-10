package costcalc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractCaptiveDeliveryCosts maps a snapshot onto the 7 fast-query cost
// columns. Guards D1: the engine used to read the pre-000406 aliases, silently
// writing 0 into cpc_captive_cost / cpc_delivery_cost / cpc_vb1..5_del_cost.
func TestExtractCaptiveDeliveryCosts(t *testing.T) {
	snap := map[string]float64{
		"CAPTIVE_COST_QLTY_LOSS":   11.1,
		"DELIVERY_COST_QLTY_LOSS":  22.2,
		"VOLUME_BUCKET_1_DEL_COST": 1.1,
		"VOLUME_BUCKET_2_DEL_COST": 2.2,
		"VOLUME_BUCKET_3_DEL_COST": 3.3,
		"VOLUME_BUCKET_4_DEL_COST": 4.4,
		"VOLUME_BUCKET_5_DEL_COST": 5.5,
		// Pre-000406 aliases: present but must be ignored.
		"COST_CAP_FINAL": 999,
		"COST_DEL_FINAL": 999,
		"VB1_DEL_COST":   999,
	}

	cc := extractCaptiveDeliveryCosts(snap)

	assert.InDelta(t, 11.1, cc.captive, 1e-9)
	assert.InDelta(t, 22.2, cc.delivery, 1e-9)
	assert.InDelta(t, 1.1, cc.vb[0], 1e-9)
	assert.InDelta(t, 2.2, cc.vb[1], 1e-9)
	assert.InDelta(t, 3.3, cc.vb[2], 1e-9)
	assert.InDelta(t, 4.4, cc.vb[3], 1e-9)
	assert.InDelta(t, 5.5, cc.vb[4], 1e-9)
}

func TestExtractCaptiveDeliveryCosts_MissingKeysAreZero(t *testing.T) {
	cc := extractCaptiveDeliveryCosts(map[string]float64{})
	assert.Zero(t, cc.captive)
	assert.Zero(t, cc.delivery)
	assert.Equal(t, [5]float64{}, cc.vb)
}

// TestSnapKeysExistInParameterSeed is the real D1 guard: every snapshot key the
// engine reads must be a code actually seeded into mst_parameter. A rename in
// the seed without a matching rename here silently zeroes the columns again.
func TestSnapKeysExistInParameterSeed(t *testing.T) {
	seed, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "migrations", "postgres", "000407_seed_oracle_142_params.up.sql"))
	require.NoError(t, err, "parameter seed migration must be readable")

	keys := append([]string{snapKeyCaptiveCost, snapKeyDeliveryCost}, snapKeysVolumeBucketDelCost[:]...)
	for _, k := range keys {
		assert.Contains(t, string(seed), "'"+k+"'",
			"snapshot key %q is not seeded in mst_parameter", k)
	}
}
