package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// newTestViper builds an isolated viper with the same defaults + env wiring
// Load() applies, minus the config-file read (tests must not depend on cwd).
func newTestViper() *viper.Viper {
	v := viper.New()
	setDefaults(v)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	return v
}

// TestBindEnvVars_SSLModeBoundToDeployedEnvName pins the binding to the env var
// name the K8s manifests actually set (DATABASE_SSLMODE). The struct key is
// database.ssl_mode, so AutomaticEnv alone would look for DATABASE_SSL_MODE and
// silently fall back to the "disable" default — connecting without TLS.
func TestBindEnvVars_SSLModeBoundToDeployedEnvName(t *testing.T) {
	t.Setenv("DATABASE_SSLMODE", "require")

	v := newTestViper()
	bindEnvVars(v)

	assert.Equal(t, "require", v.GetString("database.ssl_mode"))
}

// TestBindEnvVars_DatabaseNameBoundToDeployedEnvName guards the sibling binding
// that was keyed to the non-existent database.dbname.
func TestBindEnvVars_DatabaseNameBoundToDeployedEnvName(t *testing.T) {
	t.Setenv("DATABASE_NAME", "goapps")

	v := newTestViper()
	bindEnvVars(v)

	assert.Equal(t, "goapps", v.GetString("database.name"))
}

// TestBindEnvVars_SSLModeReachesUnmarshaledConfig proves the bound value
// survives Unmarshal into DatabaseConfig and lands in the DSN.
func TestBindEnvVars_SSLModeReachesUnmarshaledConfig(t *testing.T) {
	t.Setenv("DATABASE_SSLMODE", "require")

	v := newTestViper()
	bindEnvVars(v)

	var cfg Config
	assert.NoError(t, v.Unmarshal(&cfg))
	assert.Equal(t, "require", cfg.Database.SSLMode)
	assert.Contains(t, cfg.Database.ConnectionString(), "sslmode=require")
}

// HD-02: HRIS connection is opt-in. Without ORACLE_HRIS_* the runner must not be installed.
func TestOracleHRIS_NotConfiguredByDefault(t *testing.T) {
	v := newTestViper()
	bindEnvVars(v)

	var cfg Config
	assert.NoError(t, v.Unmarshal(&cfg))
	assert.False(t, cfg.OracleHRIS.Configured())
	assert.Equal(t, 3, cfg.OracleHRIS.MaxOpenConns, "pool kecil untuk job harian")
	assert.Equal(t, 1521, cfg.OracleHRIS.Port)
}

func TestOracleHRIS_EnvBindings(t *testing.T) {
	t.Setenv("ORACLE_USER", "ERP_USER")
	t.Setenv("ORACLE_HRIS_HOST", "hris.local")
	t.Setenv("ORACLE_HRIS_PORT", "1522")
	t.Setenv("ORACLE_HRIS_SERVICE", "ALTHARADEV")
	t.Setenv("ORACLE_HRIS_USER", "BI_HR_RO")
	t.Setenv("ORACLE_HRIS_PASSWORD", "secret")

	v := newTestViper()
	bindEnvVars(v)

	var cfg Config
	assert.NoError(t, v.Unmarshal(&cfg))
	assert.True(t, cfg.OracleHRIS.Configured())
	assert.Equal(t, "hris.local", cfg.OracleHRIS.Host)
	assert.Equal(t, 1522, cfg.OracleHRIS.Port)
	assert.Equal(t, "BI_HR_RO", cfg.OracleHRIS.User)
	// Koneksi ERP tidak tertimpa env HRIS (K5: akun terpisah).
	assert.Equal(t, "ERP_USER", cfg.Oracle.User)
}

func TestOracleConfig_Configured_RequiresAllFields(t *testing.T) {
	full := OracleConfig{Host: "h", Service: "s", User: "u", Password: "p"}
	assert.True(t, full.Configured())
	for name, c := range map[string]OracleConfig{
		"no host":     {Service: "s", User: "u", Password: "p"},
		"no service":  {Host: "h", User: "u", Password: "p"},
		"no user":     {Host: "h", Service: "s", Password: "p"},
		"no password": {Host: "h", Service: "s", User: "u"},
	} {
		assert.False(t, c.Configured(), name)
	}
}
