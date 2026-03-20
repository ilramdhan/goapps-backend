// Package config provides configuration management for IAM service using Viper.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the IAM service.
type Config struct {
	App       AppConfig       `mapstructure:"app"`
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Email     EmailConfig     `mapstructure:"email"`
	TOTP      TOTPConfig      `mapstructure:"totp"`
	Security  SecurityConfig  `mapstructure:"security"`
	CORS      CORSConfig      `mapstructure:"cors"`
	Storage   StorageConfig   `mapstructure:"storage"`
	Tracing   TracingConfig   `mapstructure:"tracing"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	Logger    LoggerConfig    `mapstructure:"logging"`
}

// CORSConfig holds CORS configuration for SSO multi-app support.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	MaxAge         int      `mapstructure:"max_age"`
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	Name    string `mapstructure:"name"`
	Version string `mapstructure:"version"`
	Env     string `mapstructure:"env"`
}

// ServerConfig holds server configuration.
type ServerConfig struct {
	GRPCPort     int           `mapstructure:"grpc_port"`
	HTTPPort     int           `mapstructure:"http_port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
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
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port"`
	Password          string        `mapstructure:"password"`
	DB                int           `mapstructure:"db"`
	SessionTTL        time.Duration `mapstructure:"session_ttl"`
	TokenBlacklistTTL time.Duration `mapstructure:"token_blacklist_ttl"`
}

// Address returns the Redis address.
func (c *RedisConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// JWTConfig holds JWT configuration.
type JWTConfig struct {
	AccessTokenSecret  string        `mapstructure:"access_token_secret"`
	RefreshTokenSecret string        `mapstructure:"refresh_token_secret"`
	AccessTokenTTL     time.Duration `mapstructure:"access_token_ttl"`
	RefreshTokenTTL    time.Duration `mapstructure:"refresh_token_ttl"`
	Issuer             string        `mapstructure:"issuer"`
}

// EmailConfig holds email/SMTP configuration.
type EmailConfig struct {
	SMTPHost     string `mapstructure:"smtp_host"`
	SMTPPort     int    `mapstructure:"smtp_port"`
	SMTPUser     string `mapstructure:"smtp_user"`
	SMTPPassword string `mapstructure:"smtp_password"`
	FromAddress  string `mapstructure:"from_address"`
	FromName     string `mapstructure:"from_name"`
	UseTLS       bool   `mapstructure:"use_tls"`
	SkipVerify   bool   `mapstructure:"skip_verify"`
}

// TOTPConfig holds TOTP 2FA configuration.
type TOTPConfig struct {
	Issuer    string `mapstructure:"issuer"`
	Digits    int    `mapstructure:"digits"`
	Period    int    `mapstructure:"period"`
	Algorithm string `mapstructure:"algorithm"`
}

// SecurityConfig holds security-related configuration.
type SecurityConfig struct {
	PasswordMinLength        int           `mapstructure:"password_min_length"`
	PasswordRequireUppercase bool          `mapstructure:"password_require_uppercase"`
	PasswordRequireLowercase bool          `mapstructure:"password_require_lowercase"`
	PasswordRequireNumber    bool          `mapstructure:"password_require_number"`
	MaxLoginAttempts         int           `mapstructure:"max_login_attempts"`
	LockoutDuration          time.Duration `mapstructure:"lockout_duration"`
	OTPExpiry                time.Duration `mapstructure:"otp_expiry"`
	ResetTokenExpiry         time.Duration `mapstructure:"reset_token_expiry"`
	SingleDeviceLogin        bool          `mapstructure:"single_device_login"`
	SessionIdleTimeout       time.Duration `mapstructure:"session_idle_timeout"`
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	RequestsPerSecond      int `mapstructure:"requests_per_second"`
	BurstSize              int `mapstructure:"burst_size"`
	LoginRequestsPerMinute int `mapstructure:"login_requests_per_minute"`
	LoginBurstSize         int `mapstructure:"login_burst_size"`
}

// TracingConfig holds Jaeger/OpenTelemetry configuration.
type TracingConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

// LoggerConfig holds logging configuration.
type LoggerConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// StorageConfig holds MinIO/S3 object storage configuration.
type StorageConfig struct {
	Endpoint           string `mapstructure:"endpoint"`
	AccessKey          string `mapstructure:"access_key"`
	SecretKey          string `mapstructure:"secret_key"`
	Bucket             string `mapstructure:"bucket"`
	BasePath           string `mapstructure:"base_path"`
	UseSSL             bool   `mapstructure:"use_ssl"`
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"` // Skip TLS verification for self-signed certs
	Region             string `mapstructure:"region"`
	PublicURL          string `mapstructure:"public_url"`
}

// Load reads configuration from file and environment variables.
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Read config file
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	// Read config file (optional, env vars can override)
	if err := v.ReadInConfig(); err != nil {
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
	v.SetDefault("app.name", "iam-service")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.env", "development")

	// Server defaults
	v.SetDefault("server.grpc_port", 50052)
	v.SetDefault("server.http_port", 8081)
	v.SetDefault("server.read_timeout", 30*time.Second)
	v.SetDefault("server.write_timeout", 30*time.Second)

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5435)
	v.SetDefault("database.user", "iam")
	v.SetDefault("database.password", "iam123")
	v.SetDefault("database.name", "iam_db")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_open_conns", 25)
	v.SetDefault("database.max_idle_conns", 5)
	v.SetDefault("database.conn_max_lifetime", 5*time.Minute)

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 1)
	v.SetDefault("redis.session_ttl", 7*24*time.Hour)
	v.SetDefault("redis.token_blacklist_ttl", 24*time.Hour)

	// JWT defaults
	v.SetDefault("jwt.access_token_secret", "change-this-in-production")
	v.SetDefault("jwt.refresh_token_secret", "change-this-in-production")
	v.SetDefault("jwt.access_token_ttl", 15*time.Minute)
	v.SetDefault("jwt.refresh_token_ttl", 7*24*time.Hour)
	v.SetDefault("jwt.issuer", "goapps-iam")

	// Email defaults
	v.SetDefault("email.smtp_host", "localhost")
	v.SetDefault("email.smtp_port", 1025)
	v.SetDefault("email.smtp_user", "")
	v.SetDefault("email.smtp_password", "")
	v.SetDefault("email.from_address", "noreply@goapps.local")
	v.SetDefault("email.from_name", "GoApps IAM")
	v.SetDefault("email.use_tls", false)
	v.SetDefault("email.skip_verify", true)

	// TOTP defaults
	v.SetDefault("totp.issuer", "GoApps")
	v.SetDefault("totp.digits", 6)
	v.SetDefault("totp.period", 30)
	v.SetDefault("totp.algorithm", "SHA1")

	// Security defaults
	v.SetDefault("security.password_min_length", 8)
	v.SetDefault("security.password_require_uppercase", true)
	v.SetDefault("security.password_require_lowercase", true)
	v.SetDefault("security.password_require_number", true)
	v.SetDefault("security.max_login_attempts", 5)
	v.SetDefault("security.lockout_duration", 15*time.Minute)
	v.SetDefault("security.otp_expiry", 5*time.Minute)
	v.SetDefault("security.reset_token_expiry", 10*time.Minute)
	v.SetDefault("security.single_device_login", true)
	v.SetDefault("security.session_idle_timeout", 2*time.Hour)

	// CORS defaults (comma-separated origins for SSO multi-app)
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:3000"})
	v.SetDefault("cors.max_age", 300)

	// Rate limit defaults
	v.SetDefault("rate_limit.requests_per_second", 100)
	v.SetDefault("rate_limit.burst_size", 200)
	v.SetDefault("rate_limit.login_requests_per_minute", 10)
	v.SetDefault("rate_limit.login_burst_size", 5)

	// Tracing defaults
	v.SetDefault("tracing.enabled", true)
	v.SetDefault("tracing.endpoint", "localhost:4317")
	v.SetDefault("tracing.insecure", true)

	// Logger defaults
	v.SetDefault("logging.level", "debug")
	v.SetDefault("logging.format", "console")

	// Storage (MinIO/S3) defaults
	v.SetDefault("storage.endpoint", "localhost:9000")
	v.SetDefault("storage.access_key", "minioadmin")
	v.SetDefault("storage.secret_key", "minioadmin")
	v.SetDefault("storage.bucket", "goapps-staging")
	v.SetDefault("storage.base_path", "iam")
	v.SetDefault("storage.use_ssl", false)
	v.SetDefault("storage.insecure_skip_verify", true)
	v.SetDefault("storage.region", "us-east-1")
	v.SetDefault("storage.public_url", "")
}

func bindEnvVars(v *viper.Viper) {
	envBindings := []struct {
		key     string
		envName string
	}{
		// Database
		{"database.host", "DATABASE_HOST"},
		{"database.port", "DATABASE_PORT"},
		{"database.user", "DATABASE_USER"},
		{"database.password", "DATABASE_PASSWORD"},
		{"database.name", "DATABASE_NAME"},
		{"database.ssl_mode", "DATABASE_SSLMODE"},
		// Redis
		{"redis.host", "REDIS_HOST"},
		{"redis.port", "REDIS_PORT"},
		{"redis.password", "REDIS_PASSWORD"},
		// JWT
		{"jwt.access_token_secret", "JWT_ACCESS_SECRET"},
		{"jwt.refresh_token_secret", "JWT_REFRESH_SECRET"},
		// Email
		{"email.smtp_host", "SMTP_HOST"},
		{"email.smtp_port", "SMTP_PORT"},
		{"email.smtp_user", "SMTP_USER"},
		{"email.smtp_password", "SMTP_PASSWORD"},
		{"email.from_address", "SMTP_FROM_ADDRESS"},
		{"email.from_name", "SMTP_FROM_NAME"},
		{"email.use_tls", "SMTP_USE_TLS"},
		{"email.skip_verify", "SMTP_SKIP_VERIFY"},
		// CORS
		{"cors.allowed_origins", "CORS_ALLOWED_ORIGINS"},
		// Tracing
		{"tracing.enabled", "TRACING_ENABLED"},
		{"tracing.endpoint", "JAEGER_ENDPOINT"},
		// App
		{"app.env", "APP_ENV"},
		{"logging.level", "LOG_LEVEL"},
		// Storage (MinIO/S3)
		{"storage.endpoint", "STORAGE_ENDPOINT"},
		{"storage.access_key", "STORAGE_ACCESS_KEY"},
		{"storage.secret_key", "STORAGE_SECRET_KEY"},
		{"storage.bucket", "STORAGE_BUCKET"},
		{"storage.base_path", "STORAGE_BASE_PATH"},
		{"storage.use_ssl", "STORAGE_USE_SSL"},
		{"storage.insecure_skip_verify", "STORAGE_INSECURE_SKIP_VERIFY"},
		{"storage.region", "STORAGE_REGION"},
		{"storage.public_url", "STORAGE_PUBLIC_URL"},
	}

	for _, binding := range envBindings {
		if err := v.BindEnv(binding.key, binding.envName); err != nil {
			fmt.Printf("Warning: failed to bind env %s: %v\n", binding.envName, err)
		}
	}
}
