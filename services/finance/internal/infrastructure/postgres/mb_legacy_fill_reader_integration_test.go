package postgres_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

func lfCaseMBLegacyFillReader_Integration(t *testing.T) {
	f := newLFFixture(t, "mblf_reader")
	h1, h2, hDel := f.head(true, ""), f.head(true, ""), f.head(true, "")
	f.exec(`UPDATE mst_mb_head SET deleted_at = NOW() WHERE mbh_id = $1`, hDel)
	f.exec(`UPDATE mst_mb_head SET mbh_dev_code = 'DEV1', mbh_denier = 2.60 WHERE mbh_id = $1`, h2)
	sp := f.spin(h1, "S1", 5, true, nil)

	csv := "entity,id,sysid,field,goapps_value,legacy_value,match_method,ambiguous,usulan,note\n" +
		"head," + h1.String() + ",1,dev,,DEV1,s,N,FILL_EMPTY,\n" +
		"head," + h2.String() + ",1,den,2.6,3,s,N,OVERWRITE,\n" +
		"head," + hDel.String() + ",1,dev,,Z,s,N,FILL_EMPTY,\n" +
		"head," + uuid.NewString() + ",1,dev,,Z,s,N,FILL_EMPTY,\n" +
		"spin," + sp.String() + ",1,shade_code,S1,S2,s,N,OVERWRITE,\n"
	p, err := mblegacyfill.ParseProposal(strings.NewReader(csv))
	require.NoError(t, err)
	rd := postgres.NewMBLegacyFillReader(postgres.NewDBFromSQL(f.raw))
	pl, err := mblegacyfill.BuildPlan(f.ctx, rd, p)
	require.NoError(t, err)
	got := []mblegacyfill.Decision{}
	for _, r := range pl.Rows {
		got = append(got, r.Decision)
	}
	require.Equal(t, []mblegacyfill.Decision{
		mblegacyfill.DecisionReject, // DEV1 held by h2
		mblegacyfill.DecisionApply,
		mblegacyfill.DecisionReject, // deleted
		mblegacyfill.DecisionReject, // missing
		mblegacyfill.DecisionApply,
	}, got)
}
