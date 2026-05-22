// Package config provides configuration management using Viper.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the finance-cost-worker service.
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
	Worker   WorkerConfig   `mapstructure:"worker"`
	Tracing  TracingConfig  `mapstructure:"tracing"`
	Logger   LoggerConfig   `mapstructure:"logger"`
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
}

// ServerConfig holds HTTP server (metrics + health) configuration.
type ServerConfig struct {
	MetricsPort int `mapstructure:"metrics_port"`
	HealthPort  int `mapstructure:"health_port"`
}

// DatabaseConfig holds PostgreSQL configuration.
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
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
}

// ConnectionString returns the PostgreSQL connection string.
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode,
	)
}

// RabbitMQConfig holds RabbitMQ connection configuration.
type RabbitMQConfig struct {
	URL            string        `mapstructure:"url"`
	PrefetchCount  int           `mapstructure:"prefetch_count"`
	ReconnectDelay time.Duration `mapstructure:"reconnect_delay"`
}

// WorkerConfig holds worker-specific configuration.
type WorkerConfig struct {
	// WorkerID identifies this worker instance in logs + chunk locks. If empty
	// at runtime, main.go generates one from hostname+pid.
	WorkerID string `mapstructure:"worker_id"`
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

	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	bindEnvVars(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "finance-cost-worker")
	v.SetDefault("app.version", "0.1.0")
	v.SetDefault("app.env", "development")

	v.SetDefault("server.metrics_port", 8093)
	v.SetDefault("server.health_port", 8083)

	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5434)
	v.SetDefault("database.user", "finance")
	v.SetDefault("database.password", "finance123")
	v.SetDefault("database.name", "finance_db")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 6)
	v.SetDefault("database.max_idle_conns", 2)
	v.SetDefault("database.conn_max_lifetime", 15*time.Minute)
	v.SetDefault("database.conn_max_idle_time", 5*time.Minute)

	v.SetDefault("rabbitmq.url", "amqp://guest:guest@localhost:5672/")
	v.SetDefault("rabbitmq.prefetch_count", 1)
	v.SetDefault("rabbitmq.reconnect_delay", 5*time.Second)

	v.SetDefault("worker.worker_id", "")

	v.SetDefault("tracing.enabled", false)
	v.SetDefault("tracing.service_name", "finance-cost-worker")
	v.SetDefault("tracing.endpoint", "localhost:4317")
	v.SetDefault("tracing.insecure", true)

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "json")
	v.SetDefault("logger.pretty_json", false)
}

func bindEnvVars(v *viper.Viper) {
	envBindings := []struct {
		key     string
		envName string
	}{
		{"database.host", "DATABASE_HOST"},
		{"database.port", "DATABASE_PORT"},
		{"database.user", "DATABASE_USER"},
		{"database.password", "DATABASE_PASSWORD"},
		{"database.name", "DATABASE_NAME"},
		{"rabbitmq.url", "RABBITMQ_URL"},
		{"worker.worker_id", "WORKER_ID"},
		{"app.env", "APP_ENV"},
		{"logger.level", "LOG_LEVEL"},
	}
	for _, b := range envBindings {
		_ = v.BindEnv(b.key, b.envName)
	}
}
