// Package config provides configuration management using Viper.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the PPC service.
type Config struct {
	App           AppConfig           `mapstructure:"app"`
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	JWT           JWTConfig           `mapstructure:"jwt"`
	CORS          CORSConfig          `mapstructure:"cors"`
	Oracle        OracleConfig        `mapstructure:"oracle"`
	ETL           ETLConfig           `mapstructure:"etl"`
	MachineSync   MachineSyncConfig   `mapstructure:"machine_sync"`
	Approval      ApprovalConfig      `mapstructure:"approval"`
	Tracing       TracingConfig       `mapstructure:"tracing"`
	Logger        LoggerConfig        `mapstructure:"logger"`
	FinanceClient FinanceClientConfig `mapstructure:"finance_client"`
	IAMClient     IAMClientConfig     `mapstructure:"iam_client"`
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	GRPCPort    int           `mapstructure:"grpc_port"`
	HTTPPort    int           `mapstructure:"http_port"`
	GRPCTimeout time.Duration `mapstructure:"grpc_timeout"`
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Name            string        `mapstructure:"name"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// ConnectionString returns the PostgreSQL connection string.
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// JWTConfig holds JWT validation configuration (shared secret with IAM).
type JWTConfig struct {
	AccessTokenSecret string `mapstructure:"access_token_secret"`
	Issuer            string `mapstructure:"issuer"`
	// ServiceSecret is a shared secret for service-to-service auth. When a
	// request carries the matching value in the x-service-secret metadata
	// header, the auth interceptor injects a synthetic SUPER_ADMIN identity
	// bypassing JWT. Empty string disables the bypass.
	ServiceSecret string `mapstructure:"service_secret"`
}

// CORSConfig holds CORS configuration for SSO multi-app support.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	MaxAge         int      `mapstructure:"max_age"`
}

// OracleConfig holds Oracle database connection configuration (read-only ETL
// source via github.com/sijms/go-ora/v2).
type OracleConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Service         string        `mapstructure:"service"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// ETLConfig holds interval + watermark settings for the Oracle ETL workers.
type ETLConfig struct {
	IntervalMinutes        int `mapstructure:"interval_minutes"`
	SOIntervalMinutes      int `mapstructure:"so_interval_minutes"`
	WatermarkBufferMinutes int `mapstructure:"watermark_buffer_minutes"`
}

// MachineSyncConfig holds the machine-sync worker interval.
type MachineSyncConfig struct {
	Interval time.Duration `mapstructure:"interval"`
}

// ApprovalConfig holds the dual-approval auto-approve window.
type ApprovalConfig struct {
	AutoApproveHours int `mapstructure:"auto_approve_hours"`
}

// FinanceClientConfig configures the gRPC client used to read finance masters
// (cost product master, routes, product grades) via the additive read-only
// finance RPCs. InternalServiceToken is sent in the x-internal-token metadata
// header so finance accepts the call without a JWT.
type FinanceClientConfig struct {
	Host                 string `mapstructure:"host"`
	Port                 int    `mapstructure:"port"`
	InternalServiceToken string `mapstructure:"internal_service_token"`
}

// IAMClientConfig configures the gRPC client used to call IAM (notifications).
// InternalServiceToken is the shared secret sent in the x-internal-token
// metadata header so IAM accepts the call without a JWT.
type IAMClientConfig struct {
	Host                 string `mapstructure:"host"`
	Port                 int    `mapstructure:"port"`
	InternalServiceToken string `mapstructure:"internal_service_token"`
}

// TracingConfig holds Jaeger/OpenTelemetry configuration.
type TracingConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	ServiceName string `mapstructure:"service_name"`
	Endpoint    string `mapstructure:"endpoint"`
	Insecure    bool   `mapstructure:"insecure"`
}

// LoggerConfig holds logging configuration.
type LoggerConfig struct {
	Level      string `mapstructure:"level"`
	Format     string `mapstructure:"format"`
	PrettyJSON bool   `mapstructure:"pretty_json"`
}

// Load reads configuration from file and environment variables.
func Load() (*Config, error) {
	v := viper.New()

	setDefaults(v)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	// Config file is optional; env vars can override.
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("app.name", "ppc-service")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.env", "development")

	// Server defaults (ports 50053/8082 — see gap D1).
	v.SetDefault("server.grpc_port", 50053)
	v.SetDefault("server.http_port", 8082)
	v.SetDefault("server.grpc_timeout", 60*time.Second)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5436)
	v.SetDefault("database.user", "ppc")
	v.SetDefault("database.password", "ppc123")
	v.SetDefault("database.name", "ppc_db")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)

	// JWT defaults (must match IAM service secret for token validation).
	v.SetDefault("jwt.access_token_secret", "change-this-in-production")
	v.SetDefault("jwt.issuer", "goapps-iam")
	v.SetDefault("jwt.service_secret", "")

	// CORS defaults (comma-separated origins for SSO multi-app).
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:3000"})
	v.SetDefault("cors.max_age", 300)

	// Oracle defaults (credentials must come from env vars — never hardcode).
	v.SetDefault("oracle.host", "localhost")
	v.SetDefault("oracle.port", 1521)
	v.SetDefault("oracle.service", "ORCLPDB1")
	v.SetDefault("oracle.user", "")
	v.SetDefault("oracle.password", "")
	v.SetDefault("oracle.max_open_conns", 5)
	v.SetDefault("oracle.conn_max_lifetime", 10*time.Minute)

	// ETL defaults (TXT production 15m, SO pending 30m, 5m watermark safety buffer).
	v.SetDefault("etl.interval_minutes", 15)
	v.SetDefault("etl.so_interval_minutes", 30)
	v.SetDefault("etl.watermark_buffer_minutes", 5)

	// Machine-sync defaults (daily sync from finance + Oracle).
	v.SetDefault("machine_sync.interval", 24*time.Hour)

	// Approval defaults (dual-approval auto-approve after 24h).
	v.SetDefault("approval.auto_approve_hours", 24)

	// Tracing defaults
	v.SetDefault("tracing.enabled", true)
	v.SetDefault("tracing.service_name", "ppc-service")
	v.SetDefault("tracing.endpoint", "localhost:4317")
	v.SetDefault("tracing.insecure", true)

	// Logger defaults
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "json")
	v.SetDefault("logger.pretty_json", false)

	// Finance gRPC client (PPC → finance for master reads).
	v.SetDefault("finance_client.host", "localhost")
	v.SetDefault("finance_client.port", 50051)
	v.SetDefault("finance_client.internal_service_token", "")

	// IAM gRPC client (PPC → IAM for notifications).
	v.SetDefault("iam_client.host", "localhost")
	v.SetDefault("iam_client.port", 50052)
	v.SetDefault("iam_client.internal_service_token", "")
}

func bindEnvVars(v *viper.Viper) {
	envBindings := []struct {
		key     string
		envName string
	}{
		// Database
		{"database.host", "PPC_DATABASE_HOST"},
		{"database.port", "PPC_DATABASE_PORT"},
		{"database.user", "PPC_DATABASE_USER"},
		{"database.password", "PPC_DATABASE_PASSWORD"},
		{"database.name", "PPC_DATABASE_NAME"},
		{"database.ssl_mode", "PPC_DATABASE_SSLMODE"},
		// JWT (shared secret with IAM)
		{"jwt.access_token_secret", "JWT_ACCESS_SECRET"},
		{"jwt.service_secret", "PPC_JWT_SERVICE_SECRET"},
		// Oracle
		{"oracle.host", "ORACLE_HOST"},
		{"oracle.port", "ORACLE_PORT"},
		{"oracle.service", "ORACLE_SERVICE"},
		{"oracle.user", "ORACLE_USER"},
		{"oracle.password", "ORACLE_PASSWORD"},
		// CORS
		{"cors.allowed_origins", "CORS_ALLOWED_ORIGINS"},
		// Tracing
		{"tracing.enabled", "TRACING_ENABLED"},
		{"tracing.endpoint", "JAEGER_ENDPOINT"},
		// App
		{"app.env", "APP_ENV"},
		{"logger.level", "LOG_LEVEL"},
		// Finance gRPC client (PPC → finance for master reads). Its token must
		// match finance's jwt.service_secret, which differs from IAM's secret —
		// so it binds to a dedicated env var (falls back to PPC_INTERNAL_TOKEN).
		{"finance_client.host", "FINANCE_GRPC_HOST"},
		{"finance_client.port", "FINANCE_GRPC_PORT"},
		{"finance_client.internal_service_token", "PPC_FINANCE_INTERNAL_TOKEN"},
		{"finance_client.internal_service_token", "PPC_INTERNAL_TOKEN"},
		// IAM gRPC client (PPC → IAM for notifications). Token must match IAM's
		// security.internal_service_token.
		{"iam_client.host", "IAM_GRPC_HOST"},
		{"iam_client.port", "IAM_GRPC_PORT"},
		{"iam_client.internal_service_token", "PPC_IAM_INTERNAL_TOKEN"},
		{"iam_client.internal_service_token", "PPC_INTERNAL_TOKEN"},
	}

	for _, binding := range envBindings {
		if err := v.BindEnv(binding.key, binding.envName); err != nil {
			// Log but don't fail — environment binding errors are non-critical.
			fmt.Printf("Warning: failed to bind env %s: %v\n", binding.envName, err)
		}
	}
}
