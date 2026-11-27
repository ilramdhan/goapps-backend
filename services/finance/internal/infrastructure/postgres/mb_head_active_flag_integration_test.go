package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// Gated by INTEGRATION_TEST=true. Fixtures prefixed ITEST-ACTFLAG2- and hard-deleted around the test.
const actFlag2Prefix = "ITEST-ACTFLAG2-"

type actFlagEnv struct {
	t        *testing.T
	ctx      context.Context
	db       *DB
	repo     *MBHeadRepository
	typeID   int32
	spinSeq  int
	products map[string]int64
	heads    map[string]uuid.UUID
}

func newActFlagEnv(t *testing.T) *actFlagEnv {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	raw, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mbRegenEnvOrDefault("TEST_DB_HOST", "localhost"), mbRegenEnvOrDefault("TEST_DB_PORT", "5434"),
		mbRegenEnvOrDefault("TEST_DB_USER", "finance"), mbRegenEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		mbRegenEnvOrDefault("TEST_DB_NAME", "finance_db")))
	require.NoError(t, err)
	require.NoError(t, mbRegenWaitForDB(raw, 10*time.Second))
	db := NewDBFromSQL(raw)
	e := &actFlagEnv{t: t, ctx: context.Background(), db: db, products: map[string]int64{}, heads: map[string]uuid.UUID{},
		repo: NewMBHeadRepository(db, NewMBCompositionRepository(db))}
	require.NoError(t, db.QueryRowContext(e.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).Scan(&e.typeID))
	e.clean()
	t.Cleanup(func() { e.clean(); _ = db.Close() })
	return e
}

func (e *actFlagEnv) exec(q string, args ...any) {
	_, err := e.db.ExecContext(e.ctx, q, args...)
	require.NoError(e.t, err)
}

func (e *actFlagEnv) clean() {
	p := actFlag2Prefix + "%"
	e.exec(`DELETE FROM cost_route_rm WHERE crm_parent_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, p)
	e.exec(`DELETE FROM mst_mb_spin WHERE mbs_parent_spin_id IS NOT NULL AND mbs_mbh_id IN (SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, p)
	e.exec(`DELETE FROM mst_mb_spin WHERE mbs_mbh_id IN (SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, p)
	e.exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, p)
	e.exec(`DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, p)
}

// head creates an active VALIDATED head with product, root spin and child spin.
func (e *actFlagEnv) head(name string) {
	code := actFlag2Prefix + name
	var pid int64
	require.NoError(e.t, e.db.QueryRowContext(e.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_source, cpm_created_by, cpm_updated_by)
		VALUES ($1,$2,$3,'MB_RECIPE','itest','itest') RETURNING cpm_product_sys_id`, code, e.typeID, code).Scan(&pid))
	hid := uuid.New()
	e.products[name], e.heads[name] = pid, hid
	e.exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_cost_product_id, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1,$2,$3,1,'VALIDATED','itest')`, hid, code, pid)
	root, child := uuid.New(), uuid.New()
	for i, sp := range []struct {
		id     uuid.UUID
		parent any
	}{{root, nil}, {child, root}} {
		e.exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_parent_spin_id, mbs_mgt_name, mbs_mb_costing, mbs_cost_product_id, mbs_status, mbs_is_active, created_by)
			VALUES ($1,$2,$3,'itest',$4,$5,'Spinning',TRUE,'itest')`, sp.id, hid, sp.parent, fmt.Sprintf("%s-S%d", code, i), pid)
	}
}

func (e *actFlagEnv) uses(parent, child string) {
	e.exec(`INSERT INTO mst_mb_composition_version (mbcv_mbh_id, mbcv_version, mbcv_validated_at, mbcv_validated_by, mbcv_seq_no,
		mbcv_group_head_id, mbcv_composition_pct, mbcv_source_type, mbcv_mb_ref_mbh_id)
		VALUES ($1,1,NOW(),'itest',(SELECT COALESCE(MAX(mbcv_seq_no),0)+1 FROM mst_mb_composition_version WHERE mbcv_mbh_id=$1 AND mbcv_version=1),$2,100,'MB',$3)`, e.heads[parent], uuid.New(), e.heads[child])
}

func (e *actFlagEnv) setActive(name string, active bool) error {
	ent, err := e.repo.GetByID(e.ctx, e.heads[name])
	require.NoError(e.t, err)
	require.NoError(e.t, ent.Update(mbhead.UpdateInput{IsActive: &active}, "itest"))
	return e.repo.Update(e.ctx, ent)
}

func (e *actFlagEnv) state(name string) (head, root, child, product bool) {
	require.NoError(e.t, e.db.QueryRowContext(e.ctx, `
		SELECT h.mbh_is_active,
		  (SELECT mbs_is_active FROM mst_mb_spin WHERE mbs_mbh_id=h.mbh_id AND mbs_parent_spin_id IS NULL),
		  (SELECT mbs_is_active FROM mst_mb_spin WHERE mbs_mbh_id=h.mbh_id AND mbs_parent_spin_id IS NOT NULL),
		  p.cpm_is_active
		FROM mst_mb_head h JOIN cost_product_master p ON p.cpm_product_sys_id=h.mbh_cost_product_id
		WHERE h.mbh_id=$1`, e.heads[name]).Scan(&head, &root, &child, &product))
	return
}

func TestActiveFlag_DeactivateGuardCascadeReactivate(t *testing.T) {
	e := newActFlagEnv(t)
	e.head("A")
	e.head("B")
	e.head("FREE")
	e.uses("A", "B")

	// B is used by active A -> rejected, names A, nothing changed.
	err := e.setActive("B", false)
	var inUse *mbhead.InUseError
	require.True(t, errors.As(err, &inUse), "got %v", err)
	require.ErrorIs(t, err, mbhead.ErrDeactivateInUse)
	require.Equal(t, []string{actFlag2Prefix + "A"}, inUse.UsedBy)
	h, r, c, p := e.state("B")
	require.True(t, h && r && c && p)

	// FREE: cascade to spins + product.
	require.NoError(t, e.setActive("FREE", false))
	h, r, c, p = e.state("FREE")
	require.False(t, h || r || c || p)

	// Reactivate: head, root spin, product on; child untouched.
	require.NoError(t, e.setActive("FREE", true))
	h, r, c, p = e.state("FREE")
	require.True(t, h && r && p)
	require.False(t, c, "child spin must stay inactive")

	// Deactivate A (nobody uses it), then B becomes deactivatable.
	require.NoError(t, e.setActive("A", false))
	require.NoError(t, e.setActive("B", false))

	// Product still referenced by an active parent's cost_route_rm stays active.
	e.head("P")
	e.head("R")
	var seqID int64
	require.NoError(t, e.db.QueryRowContext(e.ctx, `SELECT crs_seq_id FROM cost_route_seq LIMIT 1`).Scan(&seqID), "needs a cost_route_seq row")
	e.exec(`INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_product_sys_id, crm_rm_type, crm_route_rm_ratio, crm_created_by)
		VALUES ($1,$2,$3,'PRODUCT',1,'itest')`, seqID, e.products["P"], e.products["R"])
	require.NoError(t, e.setActive("R", false))
	h, r, c, p = e.state("R")
	require.False(t, h || r || c)
	require.True(t, p, "product referenced by an active route RM stays active")
}

// The data-tool writer path writes is_active with direct SQL: no guard (B is used by active A)
// and no cascade (spins/product untouched).
func TestActiveFlag_WriterPathBypassesGuardAndCascade(t *testing.T) {
	e := newActFlagEnv(t)
	e.head("A")
	e.head("B")
	e.uses("A", "B")
	e.exec(`UPDATE mst_mb_head SET mbh_is_active = FALSE WHERE mbh_id = $1`, e.heads["B"])
	e.exec(`UPDATE mst_mb_spin SET mbs_is_active = FALSE WHERE mbs_mbh_id = $1`, e.heads["B"])
	e.exec(`UPDATE cost_product_master SET cpm_is_active = FALSE WHERE cpm_product_sys_id = $1`, e.products["B"])

	tx, err := e.db.BeginTx(e.ctx, nil)
	require.NoError(t, err)
	n, err := mbasActivateHead(e.ctx, tx, e.heads["B"], "itest")
	require.NoError(t, err)
	require.EqualValues(t, 1, n)
	require.NoError(t, tx.Commit())

	h, r, c, p := e.state("B")
	require.True(t, h)
	require.False(t, r || c || p, "writer path must not cascade")
	_ = strings.TrimSpace
}
