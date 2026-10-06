package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unmarshalTest(t *testing.T) Config {
	t.Helper()
	v := newTestViper()
	bindEnvVars(v)
	var cfg Config
	require.NoError(t, v.Unmarshal(&cfg))
	return cfg
}

func TestErpDefaults_AllFlagsOffAndWriterDisabled(t *testing.T) {
	cfg := unmarshalTest(t)
	e := cfg.ERP
	assert.Equal(t, "disabled", e.WriterMode)
	assert.False(t, e.PushEnabled)
	assert.False(t, e.ValuationEnabled)
	assert.False(t, e.AdjApproveEnabled)
	assert.False(t, e.Schedule.Enabled)
	assert.False(t, e.Backtest.Enabled)
	assert.Equal(t, 30*time.Minute, e.PreviewTTL)
	assert.Equal(t, 24*time.Hour, e.DemandMaxAge)
	assert.Equal(t, 3, e.BusyRetries)
	assert.Equal(t, "view", e.DemandSource)
	assert.Equal(t, 120*time.Second, e.CallTimeout)
	assert.Equal(t, "03:00", e.Schedule.RunTime)
	assert.Equal(t, "Asia/Jakarta", e.Schedule.Timezone)
	assert.Equal(t, "previous", e.Schedule.TargetPeriod)

	o := cfg.OracleIF
	assert.Equal(t, 1521, o.Port)
	assert.Equal(t, 2, o.MaxOpenConns)
	assert.Equal(t, 1, o.MaxIdleConns)
	assert.Equal(t, 10*time.Minute, o.ConnMaxLifetime)
	assert.Equal(t, 30*time.Second, o.CallTimeout)
	assert.False(t, o.IsComplete(), "oracle_if must be incomplete by default")
	assert.ElementsMatch(t, []string{"host", "service", "user", "password"}, o.MissingFields())
}

func TestErpEnvOverrides(t *testing.T) {
	t.Setenv("ERP_WRITER_MODE", "fake")
	t.Setenv("ERP_PUSH_ENABLED", "true")
	t.Setenv("ERP_PREVIEW_TTL", "5m")
	t.Setenv("ORACLE_IF_HOST", "if-host")
	t.Setenv("ORACLE_IF_SERVICE", "SVC")
	t.Setenv("ORACLE_IF_USER", "GOAPPS_IF")
	t.Setenv("ORACLE_IF_PASSWORD", "test-only-not-a-secret")
	cfg := unmarshalTest(t)
	assert.Equal(t, "fake", cfg.ERP.WriterMode)
	assert.True(t, cfg.ERP.PushEnabled)
	assert.False(t, cfg.ERP.ValuationEnabled)
	assert.Equal(t, 5*time.Minute, cfg.ERP.PreviewTTL)
	assert.True(t, cfg.OracleIF.IsComplete())
	assert.Empty(t, cfg.OracleIF.MissingFields())
}

func TestOracleIF_IsIndependentOfLegacyOracle(t *testing.T) {
	t.Setenv("ORACLE_USER", "legacy")
	t.Setenv("ORACLE_PASSWORD", "legacy-pw")
	cfg := unmarshalTest(t)
	assert.Empty(t, cfg.OracleIF.User)
	assert.Empty(t, cfg.OracleIF.Password)
}
