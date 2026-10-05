package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

type fakeSnapshotReader struct {
	rows   []domain.AdjSnapshotRow
	err    error
	called int
}

func (f *fakeSnapshotReader) SnapshotAdjRows(context.Context, string) ([]domain.AdjSnapshotRow, error) {
	f.called++
	return f.rows, f.err
}

type fakeStdLister struct {
	rows []domain.StdRow
	err  error
}

func (f fakeStdLister) List(context.Context, int64) ([]domain.StdRow, error) { return f.rows, f.err }

type fakePreviewRepo struct {
	opened   []domain.ValuationPreview
	failed   []domain.ValuationPreview
	snapshot []domain.AdjSnapshotRow
	expected domain.BatchStatus
	openErr  error
}

func (f *fakePreviewRepo) CreateOpen(_ context.Context, p domain.ValuationPreview, _ string, expected domain.BatchStatus, rows []domain.AdjSnapshotRow) (domain.ValuationPreview, error) {
	if f.openErr != nil {
		return domain.ValuationPreview{}, f.openErr
	}
	p.ID, p.Status = "open-1", domain.PreviewOpen
	f.opened, f.snapshot, f.expected = append(f.opened, p), rows, expected
	return p, nil
}

func (f *fakePreviewRepo) CreateFailed(_ context.Context, p domain.ValuationPreview) (domain.ValuationPreview, error) {
	p.ID, p.Status = "failed-1", domain.PreviewFailed
	f.failed = append(f.failed, p)
	return p, nil
}

func (f *fakePreviewRepo) Get(context.Context, string) (domain.ValuationPreview, error) {
	return domain.ValuationPreview{}, domain.ErrPreviewNotFound
}

var previewT0 = time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)

func adjRow(head, item int64, txn, code string, qty, rate, val string) domain.AdjSnapshotRow {
	r := domain.AdjSnapshotRow{HeadSysID: head, ItemSysID: item, TxnCode: txn, ItemCode: code, GradeCode: "A", ShadeCode: "S1"}
	if qty != "" {
		r.QtyBu = nd(qty)
	}
	if rate != "" {
		r.Rate = nd(rate)
	}
	if val != "" {
		r.Val = nd(val)
	}
	return r
}

func stdOK(code, std string) domain.StdRow {
	return domain.StdRow{Key: domain.ErpKey{ItemCode: code, GradeCode: "A", ShadeCode: "S1"}, Status: domain.DeriveOK, StdCost: nd(std)}
}

type previewEnv struct {
	batches *memBatches
	reader  *fakeSnapshotReader
	repo    *fakePreviewRepo
	std     fakeStdLister
	locks   fakeLocks
	cfg     ValuationPreviewConfig
	id      int64
}

func newPreviewEnv(status domain.BatchStatus) *previewEnv {
	e := &previewEnv{
		batches: newMemBatches(),
		reader:  &fakeSnapshotReader{},
		repo:    &fakePreviewRepo{},
		locks:   fakeLocks{locked: true},
		cfg:     ValuationPreviewConfig{ValuationEnabled: true, WriterMode: domain.WriterModeFake, TTL: 10 * time.Minute},
	}
	e.id = e.batches.put(domain.BatchState{Period: "202608", Status: status})
	return e
}

func (e *previewEnv) handler() *ValuationPreviewHandler {
	return NewValuationPreviewHandler(e.batches, e.locks, e.reader, e.std, e.repo, e.cfg).
		WithClock(func() time.Time { return previewT0 })
}

func (e *previewEnv) cmd() ValuationPreviewCommand {
	return ValuationPreviewCommand{BatchID: e.id, Actor: "alice", HasPermission: true}
}

func approved() *int64 { v := int64(3); return &v }
func posted() *string  { v := "P"; return &v }

func TestValuationPreview_HappyPath(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	pa := adjRow(20, 201, domain.TxnInvAdj, "POY001", "1000", "1", "1")
	pa.HeadPostStatus = posted()
	ap := adjRow(30, 301, domain.TxnInvAdj, "POY001", "1000", "1", "1")
	ap.HeadApprStatus = approved()
	e.reader.rows = []domain.AdjSnapshotRow{
		adjRow(10, 102, domain.TxnMbInvAdj, "CMB009", "500", "", ""), // no std row: unmatched
		adjRow(10, 101, domain.TxnInvAdj, "POY001", "2000", "1.5", "3"),
		adjRow(11, 111, domain.TxnInvAdj, "POY001", "1000", "1", "1"),
		pa, ap,
		adjRow(40, 401, "OTHER", "POY001", "1", "1", "1"),
	}
	e.std.rows = []domain.StdRow{stdOK("POY001", "2.5")}

	res, err := e.handler().Handle(context.Background(), e.cmd())
	require.NoError(t, err)
	require.Len(t, e.repo.opened, 1)
	assert.Empty(t, e.repo.failed)
	assert.Equal(t, domain.StatusPushed, e.repo.expected)

	p := e.repo.opened[0]
	assert.Equal(t, domain.AdjOpValuate, p.Operation)
	assert.Equal(t, "alice", p.CreatedBy)
	assert.Equal(t, previewT0, p.CreatedAt)
	assert.Equal(t, previewT0.Add(10*time.Minute), p.ExpiresAt)
	assert.Equal(t, domain.PreviewConfirmText("202608", e.id), p.ConfirmText)
	assert.Equal(t, "202608/1", res.ConfirmText)
	assert.Equal(t, 2, p.HeadCount)
	assert.Equal(t, 3, p.ItemCount)

	// Deny list: posted and approved heads never reach the snapshot.
	for _, r := range e.repo.snapshot {
		assert.NotContains(t, []int64{20, 30, 40}, r.HeadSysID)
	}
	assert.Len(t, e.repo.snapshot, 3)
	assert.Equal(t, domain.AdjSetHash(e.repo.snapshot), p.SetHash)
	assert.Equal(t, domain.AdjExclusions{PostedHeads: 1, PostedItems: 1, ApprovedHeads: 1, ApprovedItems: 1, OtherTxnItems: 1, UnmatchedItems: 1}, res.Excluded)

	// Σ current = 3 + 1 + 0; projected = 2000/1000*2.5 + 1000/1000*2.5 + 0 (unmatched keeps current).
	assert.True(t, res.Totals.CurrentVal.Equal(decimal.RequireFromString("4")), res.Totals.CurrentVal.String())
	assert.True(t, res.Totals.ProjectedVal.Equal(decimal.RequireFromString("7.5")), res.Totals.ProjectedVal.String())
	assert.Equal(t, 2, res.Totals.ProjectedItems)
	require.Len(t, res.Totals.Heads, 2)
	assert.Equal(t, int64(10), res.Totals.Heads[0].HeadSysID)
	assert.Equal(t, 2, res.Totals.Heads[0].Items)
	assert.True(t, res.Totals.V07.OK)
	assert.Empty(t, res.Totals.V07.OffendingHeadIDs)

	var stored PreviewTotals
	require.NoError(t, json.Unmarshal(p.Totals, &stored))
	assert.True(t, stored.ProjectedVal.Equal(res.Totals.ProjectedVal))
}

func TestValuationPreview_V07RefusesWholeSet(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	e.reader.rows = []domain.AdjSnapshotRow{
		adjRow(10, 101, domain.TxnInvAdj, "POY001", "1000", "1", "1"),
		adjRow(12, 121, domain.TxnInvAdj, "PTY777", "1000", "0", "0"), // no std, current 0: offends
		adjRow(11, 111, domain.TxnInvAdj, "POY002", "1000", "1", "1"),
	}
	e.std.rows = []domain.StdRow{stdOK("POY001", "25")} // projected 25 > 20: offends

	res, err := e.handler().Handle(context.Background(), e.cmd())
	require.ErrorIs(t, err, domain.ErrValidationFailed)
	assert.Contains(t, err.Error(), "[10 12]")
	assert.Empty(t, e.repo.opened, "no OPEN preview on a V-07 refusal")
	require.Len(t, e.repo.failed, 1)
	assert.Equal(t, domain.PreviewFailed, res.Preview.Status)
	assert.False(t, res.Totals.V07.OK)
	assert.Equal(t, []int64{10, 12}, res.Totals.V07.OffendingHeadIDs)
	assert.NotEmpty(t, res.Totals.V07.Findings)
}

func TestValuationPreview_NoEligibleRows(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	pa := adjRow(20, 201, domain.TxnInvAdj, "POY001", "1", "1", "1")
	pa.HeadPostStatus = posted()
	ap := adjRow(30, 301, domain.TxnInvAdj, "POY001", "1", "1", "1")
	ap.HeadApprStatus = approved()
	for _, rows := range [][]domain.AdjSnapshotRow{nil, {pa, ap}} {
		e.reader.rows = rows
		_, err := e.handler().Handle(context.Background(), e.cmd())
		require.ErrorIs(t, err, ErrNoEligibleAdj)
	}
	assert.Empty(t, e.repo.opened)
	assert.Empty(t, e.repo.failed)
}

func TestValuationPreview_Gates(t *testing.T) {
	cases := []struct {
		name   string
		status domain.BatchStatus
		mode   domain.BatchMode
		mutate func(e *previewEnv, c *ValuationPreviewCommand)
		want   error
	}{
		{"G3 permission", domain.StatusPushed, "", func(_ *previewEnv, c *ValuationPreviewCommand) { c.HasPermission = false }, ErrValuatePermissionDenied},
		{"actor", domain.StatusPushed, "", func(_ *previewEnv, c *ValuationPreviewCommand) { c.Actor = "" }, domain.ErrActorRequired},
		{"G1 flag", domain.StatusPushed, "", func(e *previewEnv, _ *ValuationPreviewCommand) { e.cfg.ValuationEnabled = false }, domain.ErrFeatureDisabled},
		{"G2 writer disabled", domain.StatusPushed, "", func(e *previewEnv, _ *ValuationPreviewCommand) { e.cfg.WriterMode = domain.WriterModeDisabled }, domain.ErrWriterNotConfigured},
		{"G2 writer unset", domain.StatusPushed, "", func(e *previewEnv, _ *ValuationPreviewCommand) { e.cfg.WriterMode = "" }, domain.ErrWriterNotConfigured},
		{"reader absent", domain.StatusPushed, "", func(e *previewEnv, _ *ValuationPreviewCommand) { e.reader = nil }, ErrAdjSnapshotReaderNotConfigured},
		{"not found", domain.StatusPushed, "", func(_ *previewEnv, c *ValuationPreviewCommand) { c.BatchID = 999 }, domain.ErrBatchNotFound},
		{"shadow", domain.StatusValidated, domain.ModeShadow, nil, domain.ErrShadowNotPushable},
		{"not pushed", domain.StatusValidated, "", nil, ErrStepNotAllowed},
		{"already valuated", domain.StatusValuated, "", nil, ErrStepNotAllowed},
		{"G10 unlocked", domain.StatusPushed, "", func(e *previewEnv, _ *ValuationPreviewCommand) { e.locks = fakeLocks{} }, domain.ErrPeriodNotLocked},
		{"G10 error", domain.StatusPushed, "", func(e *previewEnv, _ *ValuationPreviewCommand) { e.locks = fakeLocks{err: errors.New("db")} }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newPreviewEnv(tc.status)
			if tc.mode != "" {
				e.batches = newMemBatches()
				e.id = e.batches.put(domain.BatchState{Period: "202608", Status: tc.status, Mode: tc.mode})
			}
			e.reader.rows = []domain.AdjSnapshotRow{adjRow(10, 101, domain.TxnInvAdj, "POY001", "1000", "1", "1")}
			reader := e.reader
			c := e.cmd()
			if tc.mutate != nil {
				tc.mutate(e, &c)
			}
			var h *ValuationPreviewHandler
			if e.reader == nil {
				h = NewValuationPreviewHandler(e.batches, e.locks, nil, e.std, e.repo, e.cfg)
			} else {
				h = e.handler()
			}
			_, err := h.Handle(context.Background(), c)
			require.Error(t, err)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}
			assert.Zero(t, reader.called, "no Oracle read before every gate passes")
			assert.Empty(t, e.repo.opened)
			assert.Empty(t, e.repo.failed)
		})
	}
}

func TestValuationPreview_PortErrors(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	e.reader.err = errors.New("ora read")
	_, err := e.handler().Handle(context.Background(), e.cmd())
	require.ErrorContains(t, err, "ora read")

	e = newPreviewEnv(domain.StatusPushed)
	e.reader.rows = []domain.AdjSnapshotRow{adjRow(10, 101, domain.TxnInvAdj, "POY001", "1000", "1", "1")}
	e.std.err = errors.New("std")
	_, err = e.handler().Handle(context.Background(), e.cmd())
	require.ErrorContains(t, err, "std")

	e.std.err = nil
	e.repo.openErr = domain.ErrStaleBatchStatus
	_, err = e.handler().Handle(context.Background(), e.cmd())
	require.ErrorIs(t, err, domain.ErrStaleBatchStatus)

	h := NewValuationPreviewHandler(e.batches, e.locks, e.reader, nil, e.repo, e.cfg)
	_, err = h.Handle(context.Background(), e.cmd())
	require.ErrorIs(t, err, ErrPreviewNotConfigured)
	h = NewValuationPreviewHandler(e.batches, nil, e.reader, e.std, e.repo, e.cfg)
	_, err = h.Handle(context.Background(), e.cmd())
	require.ErrorIs(t, err, ErrPeriodLockNotConfigured)
}

func TestValuationPreview_DefaultTTL(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	e.cfg.TTL = 0
	e.reader.rows = []domain.AdjSnapshotRow{adjRow(10, 101, domain.TxnInvAdj, "POY001", "1000", "1", "1")}
	_, err := e.handler().Handle(context.Background(), e.cmd())
	require.NoError(t, err)
	assert.Equal(t, previewT0.Add(defaultPreviewTTL), e.repo.opened[0].ExpiresAt)
}

func stampedRow(head, item int64, batchID int64) domain.AdjSnapshotRow {
	r := adjRow(head, item, domain.TxnInvAdj, "POY001", "1000", "1", "1")
	s := strconv.FormatInt(batchID, 10)
	r.Flex[adjFlexBatchIdx] = &s
	return r
}

func TestValuationPreview_ApproveStampedSet(t *testing.T) {
	e := newPreviewEnv(domain.StatusReconciled)
	e.reader.rows = []domain.AdjSnapshotRow{stampedRow(10, 101, e.id), stampedRow(11, 111, e.id), adjRow(12, 121, domain.TxnInvAdj, "POY001", "1", "1", "1")}
	e.std.rows = []domain.StdRow{stdOK("POY001", "2.5")}
	cmd := e.cmd()
	cmd.Operation = domain.AdjOpApprove

	_, err := e.handler().Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.Len(t, e.repo.opened, 1)
	p := e.repo.opened[0]
	assert.Equal(t, domain.AdjOpApprove, p.Operation)
	assert.Equal(t, domain.StatusReconciled, e.repo.expected)
	assert.Len(t, e.repo.snapshot, 2)
	assert.Equal(t, domain.AdjSetHash(stampedSet(e.reader.rows, e.id).rows), p.SetHash)
}

func TestValuationPreview_ApproveWrongStatus(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	cmd := e.cmd()
	cmd.Operation = domain.AdjOpApprove
	_, err := e.handler().Handle(context.Background(), cmd)
	require.ErrorIs(t, err, ErrStepNotAllowed)
}

func TestValuationPreview_Restore(t *testing.T) {
	e := newPreviewEnv(domain.StatusValuated)
	e.reader.rows = []domain.AdjSnapshotRow{stampedRow(10, 101, e.id)}
	cmd := e.cmd()
	cmd.Operation = domain.AdjOpRestore
	_, err := e.handler().Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.Len(t, e.repo.opened, 1)
	assert.Equal(t, domain.AdjOpRestore, e.repo.opened[0].Operation)

	// An approved stamped head refuses.
	e2 := newPreviewEnv(domain.StatusValuated)
	ap := stampedRow(10, 101, e2.id)
	ap.HeadApprStatus = approved()
	e2.reader.rows = []domain.AdjSnapshotRow{ap}
	_, err = e2.handler().Handle(context.Background(), cmd2(e2, domain.AdjOpRestore))
	require.Error(t, err)
	assert.Empty(t, e2.repo.opened)
}

func cmd2(e *previewEnv, op domain.AdjOperation) ValuationPreviewCommand {
	c := e.cmd()
	c.Operation = op
	return c
}

func TestValuationPreview_UnknownOp(t *testing.T) {
	e := newPreviewEnv(domain.StatusPushed)
	_, err := e.handler().Handle(context.Background(), cmd2(e, "BOGUS"))
	require.ErrorIs(t, err, ErrAdjOperationUnknown)
}
