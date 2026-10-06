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
