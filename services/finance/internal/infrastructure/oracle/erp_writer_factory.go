package oracle

// erp_writer_factory.go selects the OracleWriter implementation from config
// (plan P0-T14). It fails closed: any doubt resolves to the disabled writer.
// It never logs connection values, only field names and modes.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

// Writer factory errors. The effective writer is always usable (disabled)
// even when one of these is returned.
var (
	ErrInvalidWriterMode       = errors.New("oracle: invalid erp_integration.writer_mode")
	ErrFakeWriterInProduction  = errors.New("oracle: fake writer refused in production")
	ErrOracleIFIncomplete      = errors.New("oracle: oracle_if block incomplete")
	ErrOracleWriterUnavailable = errors.New("oracle: real oracle writer not available before P8-T2")
)

// productionEnvs are the app.env values that refuse the fake writer.
var productionEnvs = map[string]bool{"production": true, "prod": true}

// NewErpWriter returns the writer for erp.WriterMode and the mode actually in
// effect. Fallbacks to disabled are logged at error level and returned as an
// error so callers can surface them; the returned writer is never nil.
func NewErpWriter(erp config.ErpIntegrationConfig, ifCfg config.OracleIFConfig, appEnv string, logger zerolog.Logger) (domain.OracleWriter, domain.WriterMode, error) {
	mode := domain.WriterMode(strings.ToLower(strings.TrimSpace(erp.WriterMode)))
	if mode == "" {
		mode = domain.WriterModeDisabled
	}
	disabled := func(err error) (domain.OracleWriter, domain.WriterMode, error) {
		logger.Error().Err(err).Str("requested_mode", string(mode)).
			Str("effective_mode", string(domain.WriterModeDisabled)).
			Msg("erp writer: falling back to disabled")
		return NewDisabledWriter(), domain.WriterModeDisabled, err
	}
	if !mode.IsValid() {
		return disabled(fmt.Errorf("%w: %q", ErrInvalidWriterMode, erp.WriterMode))
	}
	switch mode {
	case domain.WriterModeFake:
		if productionEnvs[strings.ToLower(strings.TrimSpace(appEnv))] {
			return disabled(ErrFakeWriterInProduction)
		}
		logger.Warn().Str("effective_mode", string(mode)).Msg("erp writer: fake writer active (in-memory, no Oracle)")
		return NewFakeWriter(), mode, nil
	case domain.WriterModeOracle:
		if !ifCfg.IsComplete() {
			return disabled(fmt.Errorf("%w: missing %s", ErrOracleIFIncomplete, strings.Join(ifCfg.MissingFields(), ",")))
		}
		// The real writer lands in P8-T2; until then oracle maps to disabled.
		return disabled(ErrOracleWriterUnavailable)
	default:
		logger.Info().Str("effective_mode", string(domain.WriterModeDisabled)).Msg("erp writer: disabled")
		return NewDisabledWriter(), domain.WriterModeDisabled, nil
	}
}
