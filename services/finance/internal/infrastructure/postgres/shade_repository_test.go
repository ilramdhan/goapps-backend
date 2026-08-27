package postgres

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// TestDecideUpsertAction_ManualRowNeverOverwritten is the R8 requirement: a row
// a finance user created or edited by hand (source MANUAL) must never be
// overwritten by the Oracle sync.
func TestDecideUpsertAction_ManualRowNeverOverwritten(t *testing.T) {
	got := decideUpsertAction(nil, shade.SourceManual)
	if got != upsertActionSkipManual {
		t.Errorf("expected upsertActionSkipManual for a MANUAL row, got %v", got)
	}
}

func TestDecideUpsertAction_OracleRowIsUpdated(t *testing.T) {
	got := decideUpsertAction(nil, shade.SourceOracle)
	if got != upsertActionUpdate {
		t.Errorf("expected upsertActionUpdate for an ORACLE row, got %v", got)
	}
}

func TestDecideUpsertAction_NoExistingRow_IsInserted(t *testing.T) {
	got := decideUpsertAction(sql.ErrNoRows, "")
	if got != upsertActionInsert {
		t.Errorf("expected upsertActionInsert when no row exists, got %v", got)
	}
}

func TestDecideUpsertAction_LookupError_Propagates(t *testing.T) {
	got := decideUpsertAction(errors.New("connection reset"), "")
	if got != upsertActionLookupFailed {
		t.Errorf("expected upsertActionLookupFailed on a lookup error, got %v", got)
	}
}
