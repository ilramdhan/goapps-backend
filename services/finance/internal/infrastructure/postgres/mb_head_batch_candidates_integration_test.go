package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Gated by INTEGRATION_TEST=true. Fixtures prefixed ITEST-ACTFLAG- and hard-deleted around the test.
const actFlagPrefix = "ITEST-ACTFLAG-"

func TestBatchCandidates_ActiveFlagRules(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	raw, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mbRegenEnvOrDefault("TEST_DB_HOST", "localhost"), mbRegenEnvOrDefault("TEST_DB_PORT", "5434"),
		mbRegenEnvOrDefault("TEST_DB_USER", "finance"), mbRegenEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		mbRegenEnvOrDefault("TEST_DB_NAME", "finance_db")))
	require.NoError(t, err)
	require.NoError(t, mbRegenWaitForDB(raw, 10*time.Second))
	db := NewDBFromSQL(raw)
	defer func() { _ = db.Close() }()
	repo := NewMBHeadRepository(db, NewMBCompositionRepository(db))

	clean := func() {
		_, e := db.ExecContext(ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, actFlagPrefix+"%")
		require.NoError(t, e)
	}
	clean()
	defer clean()

	ids := map[string]uuid.UUID{}
	head := func(name string, active bool, status string, deleted bool) {
		id := uuid.New()
		ids[name] = id
		var del any
		if deleted {
			del = time.Now()
		}
		_, e := db.ExecContext(ctx, `INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_entry_status, mbh_is_active,
			mbh_current_version, deleted_at, created_by) VALUES ($1,$2,$3,$4,1,$5,'itest')`,
			id, actFlagPrefix+name, status, active, del)
		require.NoError(t, e)
	}
	ref := func(parent, child string, version int) {
		_, e := db.ExecContext(ctx, `INSERT INTO mst_mb_composition_version (mbcv_mbh_id, mbcv_version, mbcv_validated_at,
			mbcv_validated_by, mbcv_seq_no, mbcv_group_head_id, mbcv_composition_pct, mbcv_source_type, mbcv_mb_ref_mbh_id)
			VALUES ($1,$2,NOW(),'itest',(SELECT COALESCE(MAX(mbcv_seq_no),0)+1 FROM mst_mb_composition_version WHERE mbcv_mbh_id=$1 AND mbcv_version=$2),$3,100,'MB',$4)`, ids[parent], version, uuid.New(), ids[child])
		require.NoError(t, e)
	}
	head("A", true, "VALIDATED", false)   // active root
	head("I1", false, "VALIDATED", false) // inactive, used by A
	head("I2", false, "VALIDATED", false) // inactive, used by I1 (chain)
	head("ORPHAN", false, "VALIDATED", false)
	head("DEL", false, "VALIDATED", true) // deleted, used by A
	head("DRAFTREF", false, "DRAFT", false)
	head("CYC1", false, "VALIDATED", false)   // cycle I2<->CYC1 must terminate
	head("OLDREF", false, "VALIDATED", false) // only in a non-current version of A
	ref("A", "I1", 1)
	ref("I1", "I2", 1)
	ref("A", "DEL", 1)
	ref("A", "DRAFTREF", 1)
	ref("I2", "CYC1", 1)
	ref("CYC1", "I2", 1)
	ref("A", "OLDREF", 2) // current version is 1

	names := func(list []MBHeadCandidate) map[string]bool {
		m := map[string]bool{}
		for _, c := range list {
			if strings.HasPrefix(c.Code, actFlagPrefix) {
				m[strings.TrimPrefix(c.Code, actFlagPrefix)] = true
			}
		}
		return m
	}
	batch, err := repo.ListBatchCandidates(ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"A": true, "I1": true, "I2": true, "CYC1": true}, names(batch))

	push, err := repo.ListValidated(ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"A": true}, names(push))
}
