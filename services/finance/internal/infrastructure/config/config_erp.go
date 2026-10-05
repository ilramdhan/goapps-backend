package config

// config_erp.go holds the ERP cost integration configuration (plan P0-T14):
// the dedicated Oracle interface account (oracle_if) and the erp_integration
// feature flags. Every flag defaults to false and writer_mode to "disabled";
// the oracle_if password is only ever supplied through ORACLE_IF_PASSWORD.

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// OracleIFConfig is the dedicated ERP interface account (GOAPPS_IF). It is
// separate from the legacy oracle block and must never share its credentials.
type OracleIFConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Service         string        `mapstructure:"service"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	CallTimeout     time.Duration `mapstructure:"call_timeout"`
}

// IsComplete reports whether every connection field required to open the
// interface account is set. It never reveals which secret value is set.
func (c OracleIFConfig) IsComplete() bool {
	return c.Host != "" && c.Port > 0 && c.Service != "" && c.User != "" && c.Password != ""
}

// MissingFields lists the names (never the values) of absent required fields.
func (c OracleIFConfig) MissingFields() []string {
	var missing []string
	if c.Host == "" {
		missing = append(missing, "host")
	}
	if c.Port <= 0 {
		missing = append(missing, "port")
	}
	if c.Service == "" {
		missing = append(missing, "service")
	}
	if c.User == "" {
		missing = append(missing, "user")
	}
	if c.Password == "" {
		missing = append(missing, "password")
	}
	return missing
}

// ErpScheduleConfig configures the optional monthly auto-run.
type ErpScheduleConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	Cron          string `mapstructure:"cron"`
	RunDayOfMonth int    `mapstructure:"run_day_of_month"`
	RunTime       string `mapstructure:"run_time"`
	Timezone      string `mapstructure:"timezone"`
	TargetPeriod  string `mapstructure:"target_period"`
}

// ErpBacktestConfig configures the backtest harness.
type ErpBacktestConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// ErpIntegrationConfig holds the ERP integration feature flags.
type ErpIntegrationConfig struct {
	WriterMode        string            `mapstructure:"writer_mode"`
	PushEnabled       bool              `mapstructure:"push_enabled"`
	ValuationEnabled  bool              `mapstructure:"valuation_enabled"`
	AdjApproveEnabled bool              `mapstructure:"adj_approve_enabled"`
	PreviewTTL        time.Duration     `mapstructure:"preview_ttl"`
	DemandMaxAge      time.Duration     `mapstructure:"demand_max_age"`
	BusyRetries       int               `mapstructure:"busy_retries"`
	DemandSource      string            `mapstructure:"demand_source"`
	CallTimeout       time.Duration     `mapstructure:"call_timeout"`
	Schedule          ErpScheduleConfig `mapstructure:"schedule"`
	Backtest          ErpBacktestConfig `mapstructure:"backtest"`
}

// ERP integration defaults.
const (
	defaultErpWriterMode   = "disabled"
	defaultErpDemandSource = "view"
)

func setErpDefaults(v *viper.Viper) {
	// oracle_if: credentials come from env only (never hardcode).
	v.SetDefault("oracle_if.host", "")
	v.SetDefault("oracle_if.port", 1521)
	v.SetDefault("oracle_if.service", "")
	v.SetDefault("oracle_if.user", "")
	v.SetDefault("oracle_if.password", "")
	v.SetDefault("oracle_if.max_open_conns", 2)
	v.SetDefault("oracle_if.max_idle_conns", 1)
	v.SetDefault("oracle_if.conn_max_lifetime", 10*time.Minute)
	v.SetDefault("oracle_if.call_timeout", 30*time.Second)

	v.SetDefault("erp_integration.writer_mode", defaultErpWriterMode)
	v.SetDefault("erp_integration.push_enabled", false)
	v.SetDefault("erp_integration.valuation_enabled", false)
	v.SetDefault("erp_integration.adj_approve_enabled", false)
	v.SetDefault("erp_integration.preview_ttl", 30*time.Minute)
	v.SetDefault("erp_integration.demand_max_age", 24*time.Hour)
	v.SetDefault("erp_integration.busy_retries", 3)
	v.SetDefault("erp_integration.demand_source", defaultErpDemandSource)
	v.SetDefault("erp_integration.call_timeout", 120*time.Second)
	v.SetDefault("erp_integration.schedule.enabled", false)
	v.SetDefault("erp_integration.schedule.cron", "")
	v.SetDefault("erp_integration.schedule.run_day_of_month", 0)
	v.SetDefault("erp_integration.schedule.run_time", "03:00")
	v.SetDefault("erp_integration.schedule.timezone", "Asia/Jakarta")
	v.SetDefault("erp_integration.schedule.target_period", "previous")
	v.SetDefault("erp_integration.backtest.enabled", false)
}

// erpEnvBindings maps the ERP keys to their deployed env names.
func erpEnvBindings() [][2]string {
	return [][2]string{
		{"oracle_if.host", "ORACLE_IF_HOST"},
		{"oracle_if.port", "ORACLE_IF_PORT"},
		{"oracle_if.service", "ORACLE_IF_SERVICE"},
		{"oracle_if.user", "ORACLE_IF_USER"},
		{"oracle_if.password", "ORACLE_IF_PASSWORD"},
		{"erp_integration.writer_mode", "ERP_WRITER_MODE"},
		{"erp_integration.push_enabled", "ERP_PUSH_ENABLED"},
		{"erp_integration.valuation_enabled", "ERP_VALUATION_ENABLED"},
		{"erp_integration.adj_approve_enabled", "ERP_ADJ_APPROVE_ENABLED"},
		{"erp_integration.preview_ttl", "ERP_PREVIEW_TTL"},
		{"erp_integration.demand_max_age", "ERP_DEMAND_MAX_AGE"},
		{"erp_integration.busy_retries", "ERP_BUSY_RETRIES"},
		{"erp_integration.demand_source", "ERP_DEMAND_SOURCE"},
		{"erp_integration.call_timeout", "ERP_CALL_TIMEOUT"},
		{"erp_integration.schedule.enabled", "ERP_SCHEDULE_ENABLED"},
		// Not ERP_INTEGRATION_SCHEDULE: AutomaticEnv resolves that name to the
		// parent key erp_integration.schedule and blanks the whole block.
		{"erp_integration.schedule.cron", "ERP_SCHEDULE_CRON"},
		{"erp_integration.schedule.run_day_of_month", "ERP_RUN_DAY_OF_MONTH"},
		{"erp_integration.schedule.run_time", "ERP_RUN_TIME"},
		{"erp_integration.schedule.timezone", "ERP_SCHEDULE_TZ"},
		{"erp_integration.schedule.target_period", "ERP_SCHEDULE_TARGET_PERIOD"},
		{"erp_integration.backtest.enabled", "ERP_BACKTEST_ENABLED"},
	}
}

func bindErpEnvVars(v *viper.Viper) {
	for _, b := range erpEnvBindings() {
		if err := v.BindEnv(b[0], b[1]); err != nil {
			fmt.Printf("Warning: failed to bind env %s: %v\n", b[1], err)
		}
	}
}
