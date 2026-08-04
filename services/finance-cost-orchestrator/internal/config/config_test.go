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

// TestConnectionString_BinaryParametersEnabled_AppendsFlag guards the PgBouncer
// transaction-pooling fix. Without binary_parameters=yes, lib/pq splits Parse
// and Bind across two round trips and PgBouncer can swap the server connection
// in between, cross-wiring parameters between unrelated queries.
func TestConnectionString_BinaryParametersEnabled_AppendsFlag(t *testing.T) {
	c := &DatabaseConfig{
		Host: "pgbouncer.database.svc.cluster.local", Port: 5432,
		User: "app", Password: "secret", Name: "goapps", SSLMode: "disable",
		BinaryParameters: true,
	}
	assert.Equal(t,
		"host=pgbouncer.database.svc.cluster.local port=5432 user=app password=secret dbname=goapps sslmode=disable binary_parameters=yes",
		c.ConnectionString(),
	)
}

// TestConnectionString_BinaryParametersDisabled_OmitsFlag verifies the escape
// hatch for direct-to-PostgreSQL connections.
func TestConnectionString_BinaryParametersDisabled_OmitsFlag(t *testing.T) {
	c := &DatabaseConfig{
		Host: "localhost", Port: 5434,
		User: "finance", Password: "finance123", Name: "finance_db", SSLMode: "disable",
		BinaryParameters: false,
	}
	got := c.ConnectionString()
	assert.NotContains(t, got, "binary_parameters")
	assert.Equal(t,
		"host=localhost port=5434 user=finance password=finance123 dbname=finance_db sslmode=disable",
		got,
	)
}

// TestSetDefaults_BinaryParametersDefaultsTrue ensures a deployment that never
// sets the key still gets the PgBouncer-safe behavior.
func TestSetDefaults_BinaryParametersDefaultsTrue(t *testing.T) {
	v := newTestViper()
	assert.True(t, v.GetBool("database.binary_parameters"))
}

// TestBindEnvVars_BinaryParametersCanBeDisabledViaEnv verifies the documented
// DATABASE_BINARY_PARAMETERS=false override reaches the parsed config.
func TestBindEnvVars_BinaryParametersCanBeDisabledViaEnv(t *testing.T) {
	t.Setenv("DATABASE_BINARY_PARAMETERS", "false")

	v := newTestViper()
	bindEnvVars(v)

	assert.False(t, v.GetBool("database.binary_parameters"))
}

// TestBindEnvVars_SSLModeBoundToDeployedEnvName pins the binding to the env var
// name the K8s manifests actually set (DATABASE_SSLMODE). AutomaticEnv alone
// would look for DATABASE_SSL_MODE and silently miss it.
func TestBindEnvVars_SSLModeBoundToDeployedEnvName(t *testing.T) {
	t.Setenv("DATABASE_SSLMODE", "require")

	v := newTestViper()
	bindEnvVars(v)

	assert.Equal(t, "require", v.GetString("database.ssl_mode"))
}

// TestConnectionString_DoesNotLeakPasswordIntoFlagOrder is a cheap regression
// guard that the appended flag lands at the end and does not corrupt the DSN.
func TestConnectionString_DoesNotLeakPasswordIntoFlagOrder(t *testing.T) {
	c := &DatabaseConfig{
		Host: "h", Port: 1, User: "u", Password: "p", Name: "d", SSLMode: "disable",
		BinaryParameters: true,
	}
	got := c.ConnectionString()
	assert.True(t, strings.HasSuffix(got, " binary_parameters=yes"))
}
