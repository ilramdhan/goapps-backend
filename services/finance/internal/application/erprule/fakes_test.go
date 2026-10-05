package erprule

import (
	"context"
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

var fixedNow = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// fakeRules is an in-memory cst_erp_valloss_rule.
type fakeRules struct {
	rows      map[int64]*domain.VallossRule
	nextID    int64
	createErr error
	getErr    error
	listErr   error
	updateErr error
	updates   int
	lastList  domain.VallossRuleFilter
}

func newFakeRules() *fakeRules { return &fakeRules{rows: map[int64]*domain.VallossRule{}, nextID: 1} }

func (f *fakeRules) clone(r *domain.VallossRule, id int64) *domain.VallossRule {
	return domain.ReconstructVallossRule(id, r.Key(), r.Basis(), r.ValLoss(), r.IsActive(),
		r.CreatedAt(), r.CreatedBy(), r.UpdatedAt(), r.UpdatedBy())
}

func (f *fakeRules) Create(_ context.Context, r *domain.VallossRule) (*domain.VallossRule, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	for _, cur := range f.rows {
		if cur.IsActive() && cur.Key() == r.Key() {
			return nil, domain.ErrDuplicateRule
		}
	}
	id := f.nextID
	f.nextID++
	f.rows[id] = f.clone(r, id)
	return f.clone(r, id), nil
}

func (f *fakeRules) GetByID(_ context.Context, id int64) (*domain.VallossRule, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	r, ok := f.rows[id]
	if !ok {
		return nil, domain.ErrRuleNotFound
	}
	return f.clone(r, id), nil
}

func (f *fakeRules) List(_ context.Context, flt domain.VallossRuleFilter) ([]*domain.VallossRule, int64, error) {
	f.lastList = flt
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	ids := make([]int64, 0, len(f.rows))
	for id, r := range f.rows {
		if !flt.IncludeInactive && !r.IsActive() {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]*domain.VallossRule, 0, len(ids))
	for _, id := range ids {
		out = append(out, f.clone(f.rows[id], id))
	}
	return out, int64(len(out)), nil
}

func (f *fakeRules) Update(_ context.Context, r *domain.VallossRule) error {
	f.updates++
	if f.updateErr != nil {
		return f.updateErr
	}
	if _, ok := f.rows[r.ID()]; !ok {
		return domain.ErrRuleNotFound
	}
	f.rows[r.ID()] = f.clone(r, r.ID())
	return nil
}

// fakePrices is an in-memory cst_erp_sell_price.
type fakePrices struct {
	rows      map[domain.Basis]*domain.SellPrice
	getErr    error
	listErr   error
	upsertErr error
	upserts   int
}

func newFakePrices() *fakePrices { return &fakePrices{rows: map[domain.Basis]*domain.SellPrice{}} }

func clonePrice(p *domain.SellPrice) *domain.SellPrice {
	return domain.ReconstructSellPrice(p.Basis(), p.Price(), p.IsActive(), p.CreatedAt(), p.CreatedBy(), p.UpdatedAt(), p.UpdatedBy())
}

func (f *fakePrices) Get(_ context.Context, b domain.Basis) (*domain.SellPrice, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	p, ok := f.rows[b]
	if !ok {
		return nil, domain.ErrSellPriceNotFound
	}
	return clonePrice(p), nil
}

func (f *fakePrices) List(_ context.Context, includeInactive bool) ([]*domain.SellPrice, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*domain.SellPrice
	for _, b := range []domain.Basis{domain.BasisSPBSD, domain.BasisSPITY, domain.BasisSPPTY} {
		if p, ok := f.rows[b]; ok && (includeInactive || p.IsActive()) {
			out = append(out, clonePrice(p))
		}
	}
	return out, nil
}

func (f *fakePrices) Upsert(_ context.Context, p *domain.SellPrice) error {
	f.upserts++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.rows[p.Basis()] = clonePrice(p)
	return nil
}

// fakeGrades is an in-memory cost_erp_grade.
type fakeGrades struct {
	rows     map[string]*domain.Grade
	getErr   error
	listErr  error
	setErr   error
	sets     int
	lastList domain.GradeFilter
}

func newFakeGrades() *fakeGrades { return &fakeGrades{rows: map[string]*domain.Grade{}} }

func cloneGrade(g *domain.Grade) *domain.Grade {
	return domain.ReconstructGrade(g.Code(), g.Name(), g.IsActive(), g.Group())
}

func (f *fakeGrades) add(code, name string, group *domain.GradeGroup) {
	f.rows[code] = domain.ReconstructGrade(code, name, true, group)
}

func (f *fakeGrades) GetGrade(_ context.Context, code string) (*domain.Grade, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	g, ok := f.rows[code]
	if !ok {
		return nil, domain.ErrGradeNotFound
	}
	return cloneGrade(g), nil
}

func (f *fakeGrades) ListGrades(_ context.Context, flt domain.GradeFilter) ([]*domain.Grade, int64, error) {
	f.lastList = flt
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	codes := make([]string, 0, len(f.rows))
	for c, g := range f.rows {
		if flt.UnassignedOnly && g.HasGroup() {
			continue
		}
		codes = append(codes, c)
	}
	sort.Strings(codes)
	out := make([]*domain.Grade, 0, len(codes))
	for _, c := range codes {
		out = append(out, cloneGrade(f.rows[c]))
	}
	return out, int64(len(out)), nil
}

func (f *fakeGrades) SetGradeGroup(_ context.Context, g *domain.Grade) error {
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	if _, ok := f.rows[g.Code()]; !ok {
		return domain.ErrGradeNotFound
	}
	f.rows[g.Code()] = cloneGrade(g)
	return nil
}

// fakeLoader builds a RuleSet from the fakes' active rows; usage answers the
// hash question from a set of in-use hashes.
type fakeLoader struct {
	rules   *fakeRules
	prices  *fakePrices
	grades  *fakeGrades
	loadErr error
	loads   int
}

func (l *fakeLoader) LoadRuleSet(ctx context.Context) (*domain.RuleSet, error) {
	l.loads++
	if l.loadErr != nil {
		return nil, l.loadErr
	}
	var rules []domain.Rule
	if l.rules != nil {
		rs, _, _ := l.rules.List(ctx, domain.VallossRuleFilter{})
		for _, r := range rs {
			rules = append(rules, r.Rule())
		}
	}
	var prices []domain.Price
	if l.prices != nil {
		ps, _ := l.prices.List(ctx, false)
		for _, p := range ps {
			prices = append(prices, p.Value())
		}
	}
	var grades []domain.GradeAssignment
	if l.grades != nil {
		for _, g := range l.grades.rows {
			if a, ok := g.Assignment(); ok {
				grades = append(grades, a)
			}
		}
	}
	return domain.NewRuleSet(rules, prices, grades)
}

type fakeUsage struct {
	inUse   map[string]bool
	err     error
	queried []string
}

func (u *fakeUsage) IsRuleHashInUse(_ context.Context, hash string) (bool, error) {
	u.queried = append(u.queried, hash)
	if u.err != nil {
		return false, u.err
	}
	return u.inUse[hash], nil
}

// spyAudit records emitted audit rows.
type spyAudit struct {
	events []auditdomain.NewInput
	err    error
}

func (s *spyAudit) Emit(_ context.Context, in auditdomain.NewInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	s.events = append(s.events, in)
	return s.err
}

func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	require.NotEmpty(t, s)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func gg(g domain.GradeGroup) *domain.GradeGroup { return &g }

// seedRule inserts an active rule straight into the fake.
func seedRule(t *testing.T, f *fakeRules, fg, prod, group, basis, loss string) *domain.VallossRule {
	t.Helper()
	k, err := domain.NewRuleKey(fg, prod, group)
	require.NoError(t, err)
	b, err := domain.ParseBasis(basis)
	require.NoError(t, err)
	r, err := domain.NewVallossRule(k, b, dec(loss), "seed", fixedNow.Add(-time.Hour))
	require.NoError(t, err)
	created, err := f.Create(context.Background(), r)
	require.NoError(t, err)
	return created
}
