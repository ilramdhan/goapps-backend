package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	jobapp "github.com/mutugading/goapps-backend/services/finance/internal/application/bi/job"
	jobdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/job"
)

// etlStatusRepoStub embeds the interface so only ListEtlStatus needs implementing; any other
// call (e.g. List/ListLogs, which would leak the full registry) panics and fails the test.
type etlStatusRepoStub struct {
	jobdomain.Repository
	rows       []*jobdomain.EtlStatus
	err        error
	gotPrefix  string
	statusCall int
}

func (s *etlStatusRepoStub) ListEtlStatus(_ context.Context, prefix string) ([]*jobdomain.EtlStatus, error) {
	s.statusCall++
	s.gotPrefix = prefix
	return s.rows, s.err
}

func newEtlStatusHandler(t *testing.T, repo jobdomain.Repository) *BIJobHandler {
	t.Helper()
	h, err := NewBIJobHandler(nil, nil, jobapp.NewEtlStatusHandler(repo), nil, nil, nil, nil)
	require.NoError(t, err)
	return h
}

// HD-27: only ETL_HR_* is requested, timestamps map through, never-run/never-success stay unset.
func TestGetEtlStatus_MapsRowsAndScopesToHRPrefix(t *testing.T) {
	ran := time.Date(2026, 10, 27, 5, 0, 0, 0, time.UTC)
	okAt := ran.Add(-24 * time.Hour)
	repo := &etlStatusRepoStub{rows: []*jobdomain.EtlStatus{
		{JobName: "ETL_HR_OT_DAILY", LastStatus: jobdomain.StatusFailed, LastRunAt: ran, LastSuccessAt: okAt},
		{JobName: "ETL_HR_NEW"},
	}}
	resp, err := newEtlStatusHandler(t, repo).GetEtlStatus(context.Background(), &financev1.GetEtlStatusRequest{})
	require.NoError(t, err)

	assert.Equal(t, "ETL_HR_", repo.gotPrefix)
	assert.Equal(t, "200", resp.GetBase().GetStatusCode())
	require.Len(t, resp.GetData(), 2)
	assert.Equal(t, "ETL_HR_OT_DAILY", resp.GetData()[0].GetJobName())
	assert.Equal(t, "FAILED", resp.GetData()[0].GetLastStatus())
	assert.True(t, ran.Equal(resp.GetData()[0].GetLastRunAt().AsTime()))
	assert.True(t, okAt.Equal(resp.GetData()[0].GetLastSuccessAt().AsTime()))
	assert.Nil(t, resp.GetData()[1].GetLastRunAt(), "never run → unset")
	assert.Nil(t, resp.GetData()[1].GetLastSuccessAt(), "never succeeded → unset")
}

// Repo failure must not leak the raw error text to the caller.
func TestGetEtlStatus_RepoErrorNotLeaked(t *testing.T) {
	repo := &etlStatusRepoStub{err: errors.New("pq: secret-detail relation bi_job")}
	resp, err := newEtlStatusHandler(t, repo).GetEtlStatus(context.Background(), &financev1.GetEtlStatusRequest{})
	require.NoError(t, err)
	assert.NotEqual(t, "200", resp.GetBase().GetStatusCode())
	assert.NotContains(t, resp.GetBase().GetMessage(), "secret-detail")
	assert.Empty(t, resp.GetData())
}

func TestEtlStatusResponse_HasNoErrorOrConfigFields(t *testing.T) {
	d := (&financev1.EtlJobStatus{}).ProtoReflect().Descriptor()
	for _, f := range []string{"error_message", "config", "rows_affected", "triggered_by", "job_id"} {
		assert.Nil(t, d.Fields().ByName(protoreflect.Name(f)), "field %s must not exist", f)
	}
}

// HD-27: the interceptor requires finance.bi.etlstatus.view; job.view alone is not enough,
// and HR (etlstatus.view only) still cannot reach ListJobs/ListJobLogs.
func TestGetEtlStatus_PermissionGate(t *testing.T) {
	assert.Equal(t, "finance.bi.etlstatus.view", getRequiredPermission("/finance.v1.BiJobService/GetEtlStatus"))

	repo := &etlStatusRepoStub{}
	h := newEtlStatusHandler(t, repo)
	info := &grpc.UnaryServerInfo{FullMethod: "/finance.v1.BiJobService/GetEtlStatus"}
	call := func(perms, roles []string) error {
		ctx := context.WithValue(context.Background(), AuthPermissionsKey, perms)
		ctx = context.WithValue(ctx, AuthRolesKey, roles)
		_, err := PermissionInterceptor()(ctx, &financev1.GetEtlStatusRequest{}, info,
			func(ctx context.Context, req any) (any, error) {
				return h.GetEtlStatus(ctx, req.(*financev1.GetEtlStatusRequest))
			})
		return err
	}

	require.Error(t, call([]string{"finance.bi.job.view", "finance.bi.dashboard.view"}, nil))
	require.Error(t, call(nil, nil))
	assert.Zero(t, repo.statusCall, "ditolak sebelum menyentuh repo")

	require.NoError(t, call([]string{"finance.bi.etlstatus.view"}, nil))
	require.NoError(t, call(nil, []string{"SUPER_ADMIN"}))
	assert.Equal(t, 2, repo.statusCall)
}

