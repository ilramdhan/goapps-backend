package erpintegration

import (
	"context"
	"errors"
	"strings"
	"testing"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

type fakeMasterReader struct {
	items  []domain.MasterItem
	grades []domain.MasterGrade
	err    error
	calls  []string
}

func (f *fakeMasterReader) ListItems(context.Context) ([]domain.MasterItem, error) {
	f.calls = append(f.calls, "items")
	return f.items, f.err
}

func (f *fakeMasterReader) ListGrades(context.Context) ([]domain.MasterGrade, error) {
	f.calls = append(f.calls, "grades")
	return f.grades, f.err
}

// fakeMasterRepo is an in-memory replica that models the real upsert and
// apply semantics: names/active are replicated, group is never touched by
// the upsert, and the apply fills NULL groups only.
type fakeMasterRepo struct {
	items  map[string]domain.MasterItem
	grades map[string]*fakeGrade
	seed   map[string]string
	order  []string
}

type fakeGrade struct {
	name, group string
	active      bool
}

func newFakeMasterRepo() *fakeMasterRepo {
	return &fakeMasterRepo{items: map[string]domain.MasterItem{}, grades: map[string]*fakeGrade{}, seed: map[string]string{}}
}

func (r *fakeMasterRepo) UpsertItems(_ context.Context, items []domain.MasterItem) (domain.UpsertCounts, error) {
	r.order = append(r.order, "upsert_items")
	c := domain.UpsertCounts{Read: int64(len(items))}
	for _, it := range items {
		old, ok := r.items[it.Code]
		switch {
		case !ok:
			c.Inserted++
		case old != it:
			c.Updated++
		default:
			c.Unchanged++
		}
		r.items[it.Code] = it
	}
	return c, nil
}

func (r *fakeMasterRepo) UpsertGrades(_ context.Context, grades []domain.MasterGrade) (domain.UpsertCounts, error) {
	r.order = append(r.order, "upsert_grades")
	c := domain.UpsertCounts{Read: int64(len(grades))}
	for _, g := range grades {
		old, ok := r.grades[g.Code]
		switch {
		case !ok:
			c.Inserted++
			r.grades[g.Code] = &fakeGrade{name: g.Name, active: g.Active}
		case old.name != g.Name || old.active != g.Active:
			c.Updated++
			old.name, old.active = g.Name, g.Active
		default:
			c.Unchanged++
		}
	}
	return c, nil
}

func (r *fakeMasterRepo) ApplyGradeGroupSeed(context.Context) (domain.GradeGroupApplyReport, error) {
	r.order = append(r.order, "apply")
	rep := domain.GradeGroupApplyReport{SeedRows: int64(len(r.seed))}
	for code, grp := range r.seed {
		g, ok := r.grades[code]
		switch {
		case !ok:
			rep.MissingCodes = append(rep.MissingCodes, code)
		case g.group != "":
			rep.AlreadyGrouped++
		default:
			g.group = grp
			rep.AppliedNow++
		}
	}
	return rep, nil
}

type recordingAudit struct {
	inputs []auditdomain.NewInput
	err    error
}

func (a *recordingAudit) Emit(_ context.Context, in auditdomain.NewInput) error {
	a.inputs = append(a.inputs, in)
	return a.err
}

func TestMasterSync_All_InsertsUpdatesAndPreservesGroup(t *testing.T) {
	ctx := context.Background()
	repo := newFakeMasterRepo()
	repo.grades["AX"] = &fakeGrade{name: "old", group: "G9", active: true}
	repo.seed = map[string]string{"AX": "G1", "AM": "G2", "ZZ": "G3"}
	reader := &fakeMasterReader{
		items:  []domain.MasterItem{{Code: "FG1", Name: "one", Active: true}},
		grades: []domain.MasterGrade{{Code: "AX", Name: "renamed", Active: true}, {Code: "AM", Name: "new", Active: true}},
	}
	audit := &recordingAudit{}
	h := NewMasterSyncHandler(reader, repo, NewGradeGroupApplyHandler(repo, audit))

	res, err := h.Handle(ctx, MasterSubtypeAll, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if res.Items.Inserted != 1 || res.Grades.Inserted != 1 || res.Grades.Updated != 1 {
		t.Fatalf("unexpected counts: items %+v grades %+v", res.Items, res.Grades)
	}
	if repo.grades["AX"].group != "G9" || repo.grades["AX"].name != "renamed" {
		t.Fatalf("pre-set group must survive sync + apply: %+v", repo.grades["AX"])
	}
	if repo.grades["AM"].group != "G2" {
		t.Fatalf("NULL group must be filled from the seed: %+v", repo.grades["AM"])
	}
	if res.GradeGroups.AppliedNow != 1 || res.GradeGroups.AlreadyGrouped != 1 || len(res.GradeGroups.MissingCodes) != 1 {
		t.Fatalf("unexpected apply report: %+v", res.GradeGroups)
	}
	if strings.Join(repo.order, ",") != "upsert_items,upsert_grades,apply" {
		t.Fatalf("apply must run after the grade upsert: %v", repo.order)
	}
	if len(audit.inputs) != 1 || !strings.Contains(audit.inputs[0].AfterData, AuditEventGradeGroupApply) || audit.inputs[0].UserID != "tester" {
		t.Fatalf("audit not emitted as expected: %+v", audit.inputs)
	}

	rerun, err := h.Handle(ctx, MasterSubtypeAll, "")
	if err != nil {
		t.Fatal(err)
	}
	if rerun.Grades.Unchanged != 2 || rerun.GradeGroups.AppliedNow != 0 {
		t.Fatalf("re-run must be a no-op: grades %+v apply %+v", rerun.Grades, rerun.GradeGroups)
	}
	if audit.inputs[1].UserID != systemActor {
		t.Fatalf("empty actor must map to system, got %q", audit.inputs[1].UserID)
	}
}

func TestMasterSync_Subtypes(t *testing.T) {
	ctx := context.Background()
	repo := newFakeMasterRepo()
	reader := &fakeMasterReader{}
	h := NewMasterSyncHandler(reader, repo, nil)

	if _, err := h.Handle(ctx, MasterSubtypeOMItem, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Handle(ctx, MasterSubtypeOMGrade, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Handle(ctx, MasterSubtypeApplyGradeGroups, ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(repo.order, ","); got != "upsert_items,upsert_grades,apply,apply" {
		t.Fatalf("unexpected order %s", got)
	}
	if _, err := h.Handle(ctx, "bogus", ""); !errors.Is(err, domain.ErrInvalidMasterSubtype) {
		t.Fatalf("expected ErrInvalidMasterSubtype, got %v", err)
	}
}

func TestMasterSync_NotConfiguredAndErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := NewMasterSyncHandler(nil, nil, nil).Handle(ctx, MasterSubtypeAll, ""); !errors.Is(err, domain.ErrMasterSyncNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}
	if _, err := NewMasterSyncHandler(nil, newFakeMasterRepo(), nil).Handle(ctx, MasterSubtypeOMItem, ""); !errors.Is(err, domain.ErrMasterSyncNotConfigured) {
		t.Fatalf("nil reader: expected not configured, got %v", err)
	}
	boom := errors.New("ORA-12170: TNS connect timeout")
	repo := newFakeMasterRepo()
	h := NewMasterSyncHandler(&fakeMasterReader{err: boom}, repo, nil)
	if _, err := h.Handle(ctx, MasterSubtypeAll, ""); !errors.Is(err, boom) {
		t.Fatalf("expected reader error, got %v", err)
	}
	if len(repo.order) != 0 {
		t.Fatalf("nothing may be written after a read failure: %v", repo.order)
	}
}

func TestGradeGroupApply_AuditFailureIsBestEffort(t *testing.T) {
	repo := newFakeMasterRepo()
	audit := &recordingAudit{err: errors.New("audit down")}
	if _, err := NewGradeGroupApplyHandler(repo, audit).Handle(context.Background(), "u"); err != nil {
		t.Fatalf("audit failure must not fail the apply: %v", err)
	}
	var nilHandler *GradeGroupApplyHandler
	if _, err := nilHandler.Handle(context.Background(), ""); !errors.Is(err, domain.ErrMasterSyncNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}
}
