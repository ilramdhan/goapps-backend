package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// memBatches is an in-memory BatchRepository + BatchTxRunner + BatchStore.
type memBatches struct {
	mu       sync.Mutex
	batches  map[int64]domain.BatchState
	demand   map[int64][]domain.DemandLine
	coverage map[int64][]domain.CoverageLine
	nextID   int64
	saves    int
	lastInv  domain.Invalidation
	saveErr  error
}

func newMemBatches() *memBatches {
	return &memBatches{
		batches: map[int64]domain.BatchState{}, demand: map[int64][]domain.DemandLine{},
		coverage: map[int64][]domain.CoverageLine{}, nextID: 1,
	}
}

func (m *memBatches) put(st domain.BatchState) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st.ID == 0 {
		st.ID = m.nextID
		m.nextID++
	}
	if st.CreatedBy == "" {
		st.CreatedBy, st.UpdatedBy = "tester", "tester"
	}
	if st.Seq == 0 {
		st.Seq = 1
	}
	if st.Mode == "" {
		st.Mode = domain.ModeLive
	}
	m.batches[st.ID] = st
	return st.ID
}

func (m *memBatches) Create(_ context.Context, b *domain.Batch) (*domain.Batch, error) {
	st := b.State()
	id := m.put(st)
	return m.GetByID(context.Background(), id)
}

func (m *memBatches) GetByID(_ context.Context, id int64) (*domain.Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.batches[id]
	if !ok {
		return nil, domain.ErrBatchNotFound
	}
	return domain.ReconstituteBatch(st)
}

func (m *memBatches) ListByPeriod(context.Context, string) ([]*domain.Batch, error) { return nil, nil }
func (m *memBatches) FindActive(context.Context, string) (*domain.Batch, error) {
	return nil, domain.ErrBatchNotFound
}

func (m *memBatches) FindInFlight(_ context.Context, period string) (*domain.Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, st := range m.batches {
		if st.Period == period && st.Mode == domain.ModeLive && st.Status.IsInFlight() {
			return domain.ReconstituteBatch(st)
		}
	}
	return nil, domain.ErrBatchNotFound
}

func (m *memBatches) Save(_ context.Context, b *domain.Batch, expected domain.BatchStatus, inv domain.Invalidation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.saveErr != nil {
		return m.saveErr
	}
	cur, ok := m.batches[b.ID()]
	if !ok {
		return domain.ErrBatchNotFound
	}
	if cur.Status != expected {
		return errors.New("stale status")
	}
	m.batches[b.ID()] = b.State()
	m.saves++
	m.lastInv = inv
	return nil
}

func (m *memBatches) RunLocked(ctx context.Context, batchID int64, fn func(context.Context, domain.BatchStore) error) error {
	return fn(ctx, &memStore{m: m, id: batchID})
}

type memStore struct {
	m  *memBatches
	id int64
}

func (s *memStore) GetForUpdate(ctx context.Context) (*domain.Batch, error) {
	return s.m.GetByID(ctx, s.id)
}

func (s *memStore) Save(ctx context.Context, b *domain.Batch, expected domain.BatchStatus, inv domain.Invalidation) error {
	return s.m.Save(ctx, b, expected, inv)
}

func (s *memStore) ReplaceDemand(_ context.Context, lines []domain.DemandLine) (int64, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.demand[s.id] = append([]domain.DemandLine(nil), lines...)
	return int64(len(lines)), nil
}

func (s *memStore) ReplaceCoverage(_ context.Context, lines []domain.CoverageLine) (int64, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	s.m.coverage[s.id] = append([]domain.CoverageLine(nil), lines...)
	return int64(len(lines)), nil
}

func (s *memStore) ListDemand(context.Context) ([]domain.DemandLine, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	return append([]domain.DemandLine(nil), s.m.demand[s.id]...), nil
}

func (m *memBatches) state(id int64) domain.BatchState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.batches[id]
}

func (m *memBatches) summary(id int64) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(m.state(id).Summary, &out)
	return out
}

type fakeDemandReader struct {
	rows  []domain.ErpDemandRow
	err   error
	calls int
}

func (f *fakeDemandReader) LoadAdjDemand(context.Context, string) ([]domain.ErpDemandRow, error) {
	f.calls++
	return f.rows, f.err
}

type fakeProber struct {
	posted int64
	err    error
}

func (f *fakeProber) ProbeHeads(context.Context, string) (domain.AdjHeadCounts, error) {
	return domain.AdjHeadCounts{Posted: f.posted}, f.err
}

func (f *fakeProber) ProbePosted(context.Context, string) (int64, error) { return f.posted, f.err }

type fakeLocks struct {
	locked bool
	err    error
}

func (f fakeLocks) IsLocked(context.Context, string, string) (bool, error) { return f.locked, f.err }

type fakeSource struct {
	products map[ErpProductKey][]ProductCandidate
	costs    map[int64]ActualCost
	err      error
}

func (f *fakeSource) ResolveProducts(_ context.Context, keys []ErpProductKey) (map[ErpProductKey][]ProductCandidate, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[ErpProductKey][]ProductCandidate{}
	for _, k := range keys {
		if c, ok := f.products[k]; ok {
			out[k] = c
		}
	}
	return out, nil
}

func (f *fakeSource) ActualCosts(_ context.Context, _ string, ids []int64) (map[int64]ActualCost, error) {
	out := map[int64]ActualCost{}
	for _, id := range ids {
		if c, ok := f.costs[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

// memJobs is an in-memory job.Repository (only the methods the executor and
// trigger use are meaningful).
type memJobs struct {
	job.Repository
	mu       sync.Mutex
	execs    map[uuid.UUID]*job.Execution
	progress map[uuid.UUID][]int
	active   bool
}

func newMemJobs() *memJobs {
	return &memJobs{execs: map[uuid.UUID]*job.Execution{}, progress: map[uuid.UUID][]int{}}
}

func (j *memJobs) Create(_ context.Context, e *job.Execution) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.execs[e.ID()] = e
	return nil
}

func (j *memJobs) GetByID(_ context.Context, id uuid.UUID) (*job.Execution, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.execs[id]
	if !ok {
		return nil, job.ErrNotFound
	}
	return e, nil
}

func (j *memJobs) UpdateStatus(context.Context, *job.Execution) error { return nil }

func (j *memJobs) UpdateProgress(_ context.Context, id uuid.UUID, p int) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.progress[id] = append(j.progress[id], p)
	return nil
}

func (j *memJobs) HasActiveJob(context.Context, job.Type, string) (bool, error) { return j.active, nil }

type fakePublisher struct {
	err   error
	calls []string
}

func (f *fakePublisher) PublishErpIntegration(_ context.Context, jobID, subtype, _, _ string) error {
	f.calls = append(f.calls, subtype+":"+jobID)
	return f.err
}

func sortedStatuses(lines []domain.CoverageLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.ItemCode + "/" + l.ShadeCode + "=" + string(l.Status)
	}
	sort.Strings(out)
	return out
}

var testNow = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
