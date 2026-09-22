// Package formula_test provides unit tests for the formula application layer handlers.
package formula_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	formulaapp "github.com/mutugading/goapps-backend/services/finance/internal/application/formula"
	formuladomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/formula"
)

// MockRepository is a mock implementation of formula.Repository.
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(ctx context.Context, f *formuladomain.Formula) error {
	args := m.Called(ctx, f)
	return args.Error(0)
}

func (m *MockRepository) GetByID(ctx context.Context, id uuid.UUID) (*formuladomain.Formula, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*formuladomain.Formula), args.Error(1)
}

func (m *MockRepository) GetByCode(ctx context.Context, code formuladomain.Code) (*formuladomain.Formula, error) {
	args := m.Called(ctx, code)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*formuladomain.Formula), args.Error(1)
}

func (m *MockRepository) GetByResultParamID(ctx context.Context, resultParamID uuid.UUID) (*formuladomain.Formula, error) {
	args := m.Called(ctx, resultParamID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*formuladomain.Formula), args.Error(1)
}

func (m *MockRepository) List(ctx context.Context, filter formuladomain.ListFilter) ([]*formuladomain.Formula, int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*formuladomain.Formula), args.Get(1).(int64), args.Error(2)
}

func (m *MockRepository) Update(ctx context.Context, f *formuladomain.Formula) error {
	args := m.Called(ctx, f)
	return args.Error(0)
}

func (m *MockRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	args := m.Called(ctx, id, deletedBy)
	return args.Error(0)
}

func (m *MockRepository) ExistsByCode(ctx context.Context, code formuladomain.Code) (bool, error) {
	args := m.Called(ctx, code)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ListAll(ctx context.Context, filter formuladomain.ExportFilter) ([]*formuladomain.Formula, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).([]*formuladomain.Formula), args.Error(1)
}

func (m *MockRepository) ResultParamUsedByOther(ctx context.Context, resultParamID, excludeID uuid.UUID) (bool, error) {
	args := m.Called(ctx, resultParamID, excludeID)
	return args.Bool(0), args.Error(1)
}

func (m *MockRepository) ResolveParamCode(ctx context.Context, paramCode string) (*uuid.UUID, error) {
	args := m.Called(ctx, paramCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*uuid.UUID), args.Error(1)
}

func (m *MockRepository) ParamExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

// namedFormulaTypeStrings lists every non-UNSPECIFIED FormulaType's DB string, taken
// from the mst_formula_formula_type_check CHECK constraint (migration 000402). The
// save path must accept every one of these, not just CALCULATION/SQL_QUERY/CONSTANT.
func namedFormulaTypeStrings() []string {
	return []string{
		"CALCULATION", "SQL_QUERY", "CONSTANT",
		"CONDITIONAL", "LOOKUP", "RM_LOOKUP",
		"FROM_MARKETING", "INTERMINGLING", "SNAPSHOT",
		"PENDING", "INITIAL_VALUE",
	}
}

func TestCreateHandler_Handle_AcceptsAllValidFormulaTypes(t *testing.T) {
	resultParamID := uuid.New()

	for _, ft := range namedFormulaTypeStrings() {
		t.Run(ft, func(t *testing.T) {
			mockRepo := new(MockRepository)
			handler := formulaapp.NewCreateHandler(mockRepo)
			ctx := context.Background()

			cmd := formulaapp.CreateCommand{
				FormulaCode:   "F_TEST_" + ft,
				FormulaName:   "Test formula",
				FormulaType:   ft,
				Expression:    "1",
				ResultParamID: resultParamID.String(),
				CreatedBy:     "admin",
			}

			mockRepo.On("ExistsByCode", ctx, mock.AnythingOfType("formula.Code")).Return(false, nil)
			mockRepo.On("ParamExistsByID", ctx, resultParamID).Return(true, nil)
			mockRepo.On("ResultParamUsedByOther", ctx, resultParamID, uuid.Nil).Return(false, nil)
			mockRepo.On("Create", ctx, mock.AnythingOfType("*formula.Formula")).Return(nil)
			mockRepo.On("GetByID", ctx, mock.AnythingOfType("uuid.UUID")).Return(&formuladomain.Formula{}, nil)

			result, err := handler.Handle(ctx, cmd)

			require.NoError(t, err, "formula type %q must be accepted by the create path", ft)
			assert.NotNil(t, result)
		})
	}
}

func TestCreateHandler_Handle_RejectsInvalidFormulaType(t *testing.T) {
	mockRepo := new(MockRepository)
	handler := formulaapp.NewCreateHandler(mockRepo)
	ctx := context.Background()

	cmd := formulaapp.CreateCommand{
		FormulaCode:   "F_TEST_BAD",
		FormulaName:   "Test formula",
		FormulaType:   "NOT_A_REAL_TYPE",
		Expression:    "1",
		ResultParamID: uuid.New().String(),
		CreatedBy:     "admin",
	}

	_, err := handler.Handle(ctx, cmd)

	require.Error(t, err)
	assert.ErrorIs(t, err, formuladomain.ErrInvalidFormulaType)
}

func TestUpdateHandler_Handle_AcceptsAllValidFormulaTypes(t *testing.T) {
	for _, ft := range namedFormulaTypeStrings() {
		t.Run(ft, func(t *testing.T) {
			resultParamID := uuid.New()

			existing, err := formuladomain.NewFormula(
				mustNewCode(t, "F_EXISTING"),
				"Existing formula",
				formuladomain.TypeCalculation,
				"1",
				resultParamID,
				nil,
				"",
				"admin",
			)
			require.NoError(t, err)

			mockRepo := new(MockRepository)
			handler := formulaapp.NewUpdateHandler(mockRepo)
			ctx := context.Background()

			mockRepo.On("GetByID", ctx, existing.ID()).Return(existing, nil)
			mockRepo.On("Update", ctx, mock.AnythingOfType("*formula.Formula")).Return(nil)

			ftCopy := ft
			cmd := formulaapp.UpdateCommand{
				FormulaID:   existing.ID().String(),
				FormulaType: &ftCopy,
				UpdatedBy:   "admin",
			}

			result, err := handler.Handle(ctx, cmd)

			require.NoError(t, err, "formula type %q must be accepted by the update path", ft)
			assert.NotNil(t, result)
		})
	}
}

func mustNewCode(t *testing.T, s string) formuladomain.Code {
	t.Helper()
	code, err := formuladomain.NewCode(s)
	require.NoError(t, err)
	return code
}
