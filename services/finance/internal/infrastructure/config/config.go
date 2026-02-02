// Package config provides configuration management using Viper.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the service.
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Tracing  TracingConfig  `mapstructure:"tracing"`
	Logger   LoggerConfig   `mapstructure:"logger"`
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

// RedisConfig holds Redis configuration.
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Address returns the Redis address.
func (c *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
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
	configPath := ""
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
	}

	// Read config file (optional, env vars can override)
	if err := v.ReadInConfig(); err != nil {
		// Config file is optional
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Environment variables override config file
	v.SetEnvPrefix("")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Bind specific environment variables
	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("app.name", "finance-service")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.env", "development")

	// Server defaults
	v.SetDefault("server.grpc_port", 50051)
	v.SetDefault("server.http_port", 8080)
	v.SetDefault("server.grpc_timeout", 30*time.Second)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5434)
	v.SetDefault("database.user", "finance")
	v.SetDefault("database.password", "finance123")
	v.SetDefault("database.name", "finance_db")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	// Tracing defaults
	v.SetDefault("tracing.enabled", true)
	v.SetDefault("tracing.service_name", "finance-service")
	v.SetDefault("tracing.endpoint", "localhost:4317")
	v.SetDefault("tracing.insecure", true)

	// Logger defaults
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "json")
	v.SetDefault("logger.pretty_json", false)
}

func bindEnvVars(v *viper.Viper) {
	// These bindings are best-effort; errors are unlikely and non-critical
	// as viper will still read from environment via AutomaticEnv.
	envBindings := []struct {
		key     string
		envName string
	}{
		// Database
		{"database.host", "DATABASE_HOST"},
		{"database.port", "DATABASE_PORT"},
		{"database.user", "DATABASE_USER"},
		{"database.password", "DATABASE_PASSWORD"},
		{"database.dbname", "DATABASE_NAME"},
		{"database.sslmode", "DATABASE_SSLMODE"},
		// Redis
		{"redis.host", "REDIS_HOST"},
		{"redis.port", "REDIS_PORT"},
		{"redis.password", "REDIS_PASSWORD"},
		// Tracing
		{"tracing.enabled", "TRACING_ENABLED"},
		{"tracing.endpoint", "JAEGER_ENDPOINT"},
		// App
		{"app.environment", "APP_ENV"},
		{"logger.level", "LOG_LEVEL"},
	}

	for _, binding := range envBindings {
		if err := v.BindEnv(binding.key, binding.envName); err != nil {
			// Log but don't fail - environment binding errors are non-critical
			fmt.Printf("Warning: failed to bind env %s: %v\n", binding.envName, err)
		}
	}
}
