package costimportetl

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/storage"
)

// recordingStaging is a StagingPipeline fake that records which layers were
// invoked (in order) and returns configurable collected errors, so the handler's
// orchestration can be asserted without a database.
type recordingStaging struct {
	copyCalls    []string
	resolveCalls []string
	collectErrs  []StagingError
	collectErr   error
	cleanupCalls int
}

func (s *recordingStaging) copyN(token string) (int64, error) {
	s.copyCalls = append(s.copyCalls, token)
	return 1, nil
}

func (s *recordingStaging) resolveN(token string) (int, error) {
	s.resolveCalls = append(s.resolveCalls, token)
	return 1, nil
}

func (s *recordingStaging) CopyStagingProductMaster(_ context.Context, _ int64, _ RowProducer) (int64, error) {
	return s.copyN(tokenProductMaster)
}

func (s *recordingStaging) CopyStagingProductParameter(_ context.Context, _ int64, _ RowProducer) (int64, error) {
	return s.copyN(tokenProductParameter)
}

func (s *recordingStaging) CopyStagingApplicableParam(_ context.Context, _ int64, _ RowProducer) (int64, error) {
	return s.copyN(tokenApplicableParam)
}

func (s *recordingStaging) CopyStagingRouteHead(_ context.Context, _ int64, _ RowProducer) (int64, error) {
	return s.copyN(tokenRouteHead)
}

func (s *recordingStaging) CopyStagingRouteSeq(_ context.Context, _ int64, _ RowProducer) (int64, error) {
	return s.copyN(tokenRouteSeq)
}

func (s *recordingStaging) CopyStagingRouteRM(_ context.Context, _ int64, _ RowProducer) (int64, error) {
	return s.copyN(tokenRouteRM)
}

func (s *recordingStaging) ResolveLayer1Products(_ context.Context, _ int64, _ string) (int, error) {
	return s.resolveN(tokenProductMaster)
}

func (s *recordingStaging) ResolveLayer2Params(_ context.Context, _ int64, _ string) (int, error) {
	return s.resolveN(tokenProductParameter)
}

func (s *recordingStaging) ResolveLayer3Applicable(_ context.Context, _ int64, _ string) (int, error) {
	return s.resolveN(tokenApplicableParam)
}

func (s *recordingStaging) ResolveLayer4RouteHead(_ context.Context, _ int64, _ string) (int, error) {
	return s.resolveN(tokenRouteHead)
}

func (s *recordingStaging) ResolveLayer5RouteSeq(_ context.Context, _ int64, _ string) (int, error) {
	return s.resolveN(tokenRouteSeq)
}

func (s *recordingStaging) ResolveLayer6RouteRM(_ context.Context, _ int64, _ string) (int, error) {
	return s.resolveN(tokenRouteRM)
}

func (s *recordingStaging) MasterLookupCandidates(_ context.Context, _ int64) ([]MasterLookupCandidate, error) {
	return nil, nil
}

func (s *recordingStaging) RejectMasterLookupValues(_ context.Context, _ int64, _ []MasterLookupCandidate) (int, error) {
	return 0, nil
}

func (s *recordingStaging) CollectErrors(_ context.Context, _ int64) ([]StagingError, error) {
	return s.collectErrs, s.collectErr
}

func (s *recordingStaging) CleanupStaging(_ context.Context, _ int64) error {
	s.cleanupCalls++
	return nil
}

// fakeStorage embeds storage.Service (nil) and overrides only the two methods
// the handler exercises, so unrelated interface methods are never called.
type fakeStorage struct {
	storage.Service
	obj     []byte
	getErr  error
	putKeys []string
}

func (f *fakeStorage) GetObjectStream(_ context.Context, _ string) (io.ReadCloser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return io.NopCloser(bytes.NewReader(f.obj)), nil
}

func (f *fakeStorage) PutObject(_ context.Context, key string, reader io.Reader, _ int64, _ string) error {
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return err
	}
	f.putKeys = append(f.putKeys, key)
	return nil
}

// fakeJobRepo serves a single in-memory job and records Update calls.
type fakeJobRepo struct {
	job     *costimportjob.CostImportJob
	getErr  error
	updates int
}

func (r *fakeJobRepo) Create(_ context.Context, _ *costimportjob.CostImportJob) error { return nil }

func (r *fakeJobRepo) GetByID(_ context.Context, _ int64) (*costimportjob.CostImportJob, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.job, nil
}

func (r *fakeJobRepo) Update(_ context.Context, _ *costimportjob.CostImportJob) error {
	r.updates++
	return nil
}

func (r *fakeJobRepo) List(_ context.Context, _, _ string, _, _ int) ([]*costimportjob.CostImportJob, int64, error) {
	return nil, 0, nil
}

// makeZip builds a minimal valid .zip whose entries carry a header + one data
// row, enough for openContainer to succeed (the fake staging ignores the rows).
func makeZip(t *testing.T, entries ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte("col_a,col_b\nval_a,val_b\n"))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func newJob(entity, fileKey string) *costimportjob.CostImportJob {
	return costimportjob.NewJob(entity, fileKey, "tester", "")
}

// TestHandle_ParamsOnly_Done verifies a clean params-only import runs only the
// two parameter layers, finishes DONE, writes no error report, and cleans up.
func TestHandle_ParamsOnly_Done(t *testing.T) {
	staging := &recordingStaging{}
	store := &fakeStorage{obj: makeZip(t, "product_parameters.csv", "applicable_params.csv")}
	job := newJob(costimportjob.EntityBulkParamsOnly, "imports/bulk_params_only/x.zip")
	jobRepo := &fakeJobRepo{job: job}

	h := NewHandler(jobRepo, staging, store, nil, zerolog.Nop())
	require.NoError(t, h.Handle(context.Background(), 42, costimportjob.EntityBulkParamsOnly))

	require.Equal(t, []string{tokenProductParameter, tokenApplicableParam}, staging.copyCalls)
	require.Equal(t, []string{tokenProductParameter, tokenApplicableParam}, staging.resolveCalls)
	require.Equal(t, costimportjob.StatusDone, job.Status())
	require.Equal(t, 1, staging.cleanupCalls)
	require.Empty(t, store.putKeys, "no error report when there are no row errors")
	require.Empty(t, job.ErrorFile())
}

// TestHandle_ParamsOnly_Partial verifies that captured row-level errors flip the
// job to PARTIAL, generate an error report, and attach it to the job.
func TestHandle_ParamsOnly_Partial(t *testing.T) {
	staging := &recordingStaging{
		collectErrs: []StagingError{
			{Sheet: "product_parameter", RowNumber: 2, KeyInfo: "L-404", Message: "produk tidak dikenal: L-404"},
		},
	}
	store := &fakeStorage{obj: makeZip(t, "product_parameters.csv", "applicable_params.csv")}
	job := newJob(costimportjob.EntityBulkParamsOnly, "imports/bulk_params_only/y.zip")
	jobRepo := &fakeJobRepo{job: job}

	h := NewHandler(jobRepo, staging, store, nil, zerolog.Nop())
	require.NoError(t, h.Handle(context.Background(), 7, costimportjob.EntityBulkParamsOnly))

	require.Equal(t, costimportjob.StatusPartial, job.Status())
	require.Equal(t, 1, job.Failed())
	require.Len(t, store.putKeys, 1, "an error report must be uploaded")
	require.Equal(t, "imports/bulk_params_only/7/error-report.xlsx", store.putKeys[0])
	require.Equal(t, store.putKeys[0], job.ErrorFile())
	require.Equal(t, 1, staging.cleanupCalls)
}

// TestHandle_ProductRouting_AllLayers verifies the product+routing kind runs all
// six layers in dependency order.
func TestHandle_ProductRouting_AllLayers(t *testing.T) {
	staging := &recordingStaging{}
	store := &fakeStorage{obj: makeZip(t, "product_master.csv", "route_head.csv")}
	job := newJob(costimportjob.EntityBulkProductRouting, "imports/bulk_product_routing/z.zip")
	jobRepo := &fakeJobRepo{job: job}

	h := NewHandler(jobRepo, staging, store, nil, zerolog.Nop())
	require.NoError(t, h.Handle(context.Background(), 99, costimportjob.EntityBulkProductRouting))

	want := []string{
		tokenProductMaster, tokenProductParameter, tokenApplicableParam,
		tokenRouteHead, tokenRouteSeq, tokenRouteRM,
	}
	require.Equal(t, want, staging.copyCalls)
	require.Equal(t, want, staging.resolveCalls)
	require.Equal(t, costimportjob.StatusDone, job.Status())
	require.Equal(t, 1, staging.cleanupCalls)
}

// TestHandle_FetchError_Fails verifies that a storage fetch failure fails the
// job, returns the error, and still cleans up staging.
func TestHandle_FetchError_Fails(t *testing.T) {
	staging := &recordingStaging{}
	store := &fakeStorage{getErr: errors.New("minio unreachable")}
	job := newJob(costimportjob.EntityBulkParamsOnly, "imports/bulk_params_only/x.zip")
	jobRepo := &fakeJobRepo{job: job}

	h := NewHandler(jobRepo, staging, store, nil, zerolog.Nop())
	err := h.Handle(context.Background(), 5, costimportjob.EntityBulkParamsOnly)

	require.Error(t, err)
	require.Equal(t, costimportjob.StatusFailed, job.Status())
	require.Empty(t, staging.copyCalls, "no layers run when the file cannot be fetched")
	require.Equal(t, 1, staging.cleanupCalls, "staging is cleaned up even on fatal error")
}
