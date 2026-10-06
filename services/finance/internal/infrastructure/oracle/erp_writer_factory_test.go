package oracle

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
)

const factoryTestPassword = "Sup3r-Secret-IF-pw" //nolint:gosec // synthetic test value, asserted absent from logs

func completeIF() config.OracleIFConfig {
	return config.OracleIFConfig{Host: "if-host", Port: 1521, Service: "SVC", User: "GOAPPS_IF", Password: factoryTestPassword}
}

func TestNewErpWriter_Modes(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		env      string
		ifCfg    config.OracleIFConfig
		wantMode domain.WriterMode
		wantErr  error
		wantFake bool
	}{
		{"default empty is disabled", "", "development", completeIF(), domain.WriterModeDisabled, nil, false},
		{"disabled", "disabled", "production", completeIF(), domain.WriterModeDisabled, nil, false},
		{"fake in dev", "fake", "development", config.OracleIFConfig{}, domain.WriterModeFake, nil, true},
		{"fake in staging", "FAKE", "staging", config.OracleIFConfig{}, domain.WriterModeFake, nil, true},
		{"fake refused in production", "fake", "production", completeIF(), domain.WriterModeDisabled, ErrFakeWriterInProduction, false},
		{"fake refused in prod alias", "fake", " PROD ", completeIF(), domain.WriterModeDisabled, ErrFakeWriterInProduction, false},
		{"oracle without IF block", "oracle", "production", config.OracleIFConfig{Port: 1521}, domain.WriterModeDisabled, ErrOracleIFIncomplete, false},
		{"oracle with IF block maps to disabled", "oracle", "production", completeIF(), domain.WriterModeDisabled, ErrOracleWriterUnavailable, false},
		{"invalid mode", "live", "development", completeIF(), domain.WriterModeDisabled, ErrInvalidWriterMode, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			w, mode, err := NewErpWriter(config.ErpIntegrationConfig{WriterMode: tc.mode}, tc.ifCfg, tc.env, zerolog.New(&buf))
			if w == nil {
				t.Fatal("writer must never be nil")
			}
			if mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tc.wantMode)
			}
			if tc.wantErr == nil && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			_, isFake := w.(*FakeWriter)
			if isFake != tc.wantFake {
				t.Fatalf("fake = %v, want %v", isFake, tc.wantFake)
			}
			if !tc.wantFake {
				if _, err := w.LockBatch(context.Background(), 1); !errors.Is(err, domain.ErrWriterDisabled) {
					t.Fatalf("fallback writer must be disabled, got %v", err)
				}
			}
			if strings.Contains(buf.String(), factoryTestPassword) || (err != nil && strings.Contains(err.Error(), factoryTestPassword)) {
				t.Fatal("password leaked into log or error")
			}
		})
	}
}

func TestNewErpWriter_MissingFieldsLoggedByNameOnly(t *testing.T) {
	var buf bytes.Buffer
	ifCfg := config.OracleIFConfig{Host: "secret-host-value", Port: 1521, User: "GOAPPS_IF", Password: factoryTestPassword}
	_, _, err := NewErpWriter(config.ErpIntegrationConfig{WriterMode: "oracle"}, ifCfg, "production", zerolog.New(&buf))
	if !errors.Is(err, ErrOracleIFIncomplete) || !strings.Contains(err.Error(), "service") {
		t.Fatalf("expected missing service, got %v", err)
	}
	out := buf.String() + err.Error()
	for _, secret := range []string{factoryTestPassword, "secret-host-value", "GOAPPS_IF"} {
		if strings.Contains(out, secret) {
			t.Fatalf("config value %q leaked: %s", secret, out)
		}
	}
	if !strings.Contains(buf.String(), `"level":"error"`) {
		t.Fatalf("fallback must log at error level: %s", buf.String())
	}
}
