// Package session_test provides unit tests for application layer session handlers.
package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	appsession "github.com/mutugading/goapps-backend/services/iam/internal/application/session"
	domainsession "github.com/mutugading/goapps-backend/services/iam/internal/domain/session"
)

// =============================================================================
// Mock Repository
// =============================================================================

// MockSessionRepository is a mock implementation of session.Repository.
type MockSessionRepository struct {
	mock.Mock
}

func (m *MockSessionRepository) Create(ctx context.Context, s *domainsession.Session) error {
	args := m.Called(ctx, s)
	return args.Error(0)
}

func (m *MockSessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*domainsession.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainsession.Session), args.Error(1)
}

func (m *MockSessionRepository) GetByRefreshToken(ctx context.Context, tokenHash string) (*domainsession.Session, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainsession.Session), args.Error(1)
}

func (m *MockSessionRepository) GetByTokenID(ctx context.Context, tokenID string) (*domainsession.Session, error) {
	args := m.Called(ctx, tokenID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainsession.Session), args.Error(1)
}

func (m *MockSessionRepository) GetActiveByUserID(ctx context.Context, userID uuid.UUID) (*domainsession.Session, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domainsession.Session), args.Error(1)
}

func (m *MockSessionRepository) Update(ctx context.Context, s *domainsession.Session) error {
	args := m.Called(ctx, s)
	return args.Error(0)
}

func (m *MockSessionRepository) Revoke(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockSessionRepository) RevokeByTokenID(ctx context.Context, tokenID string) error {
	args := m.Called(ctx, tokenID)
	return args.Error(0)
}

func (m *MockSessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

func (m *MockSessionRepository) ListActive(ctx context.Context, params domainsession.ListParams) ([]*domainsession.Info, int64, error) {
	args := m.Called(ctx, params)
	return args.Get(0).([]*domainsession.Info), args.Get(1).(int64), args.Error(2)
}

func (m *MockSessionRepository) CleanupExpired(ctx context.Context) (int, error) {
	args := m.Called(ctx)
	return args.Int(0), args.Error(1)
}

// =============================================================================
// ListHandler
// =============================================================================

func TestListHandler(t *testing.T) {
	t.Run("success - returns paginated sessions", func(t *testing.T) {
		mockRepo := new(MockSessionRepository)
		handler := appsession.NewListHandler(mockRepo)
		ctx := context.Background()

		now := time.Now()
		expires := now.Add(24 * time.Hour)
		userID1 := uuid.New()
		userID2 := uuid.New()

		sessions := []*domainsession.Info{
			{
				SessionID:   uuid.New(),
				UserID:      userID1,
				Username:    "john",
				FullName:    "John Doe",
				DeviceInfo:  "Chrome/120",
				IPAddress:   "192.168.1.1",
				ServiceName: "iam",
				CreatedAt:   now,
				ExpiresAt:   expires,
				RevokedAt:   nil,
			},
			{
				SessionID:   uuid.New(),
				UserID:      userID2,
				Username:    "jane",
				FullName:    "Jane Smith",
				DeviceInfo:  "Firefox/121",
				IPAddress:   "192.168.1.2",
				ServiceName: "iam",
				CreatedAt:   now,
				ExpiresAt:   expires,
				RevokedAt:   nil,
			},
		}

		mockRepo.On("ListActive", ctx, mock.AnythingOfType("session.ListParams")).Return(
			sessions,
			int64(2),
			nil,
		)

		query := appsession.ListQuery{Page: 1, PageSize: 10}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Sessions, 2)
		assert.Equal(t, int64(2), result.TotalItems)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(1), result.TotalPages)
		assert.Equal(t, "john", result.Sessions[0].Username)
		assert.Equal(t, "jane", result.Sessions[1].Username)
		mockRepo.AssertExpectations(t)
	})

	t.Run("default pagination when values are zero", func(t *testing.T) {
		mockRepo := new(MockSessionRepository)
		handler := appsession.NewListHandler(mockRepo)
		ctx := context.Background()

		mockRepo.On("ListActive", ctx, mock.MatchedBy(func(p domainsession.ListParams) bool {
			return p.Page == 1 && p.PageSize == 10
		})).Return(
			[]*domainsession.Info{},
			int64(0),
			nil,
		)

		query := appsession.ListQuery{Page: 0, PageSize: 0}
		result, err := handler.Handle(ctx, query)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.Sessions)
		assert.Equal(t, int32(1), result.CurrentPage)
		assert.Equal(t, int32(10), result.PageSize)
		assert.Equal(t, int32(0), result.TotalPages)
		mockRepo.AssertExpectations(t)
	})
}

// =============================================================================
// RevokeHandler
// =============================================================================

func TestRevokeHandler(t *testing.T) {
	t.Run("success - revokes session", func(t *testing.T) {
		mockRepo := new(MockSessionRepository)
		handler := appsession.NewRevokeHandler(mockRepo)
		ctx := context.Background()

		sessionID := uuid.New()
		mockRepo.On("Revoke", ctx, sessionID).Return(nil)

		cmd := appsession.RevokeCommand{SessionID: sessionID.String()}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockSessionRepository)
		handler := appsession.NewRevokeHandler(mockRepo)
		ctx := context.Background()

		cmd := appsession.RevokeCommand{SessionID: "not-a-valid-uuid"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid session ID")
	})
}

// =============================================================================
// RevokeAllHandler
// =============================================================================

func TestRevokeAllHandler(t *testing.T) {
	t.Run("success - revokes all sessions for user", func(t *testing.T) {
		mockRepo := new(MockSessionRepository)
		handler := appsession.NewRevokeAllHandler(mockRepo)
		ctx := context.Background()

		userID := uuid.New()
		mockRepo.On("RevokeAllForUser", ctx, userID).Return(nil)

		cmd := appsession.RevokeAllCommand{UserID: userID.String()}
		err := handler.Handle(ctx, cmd)

		assert.NoError(t, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("error - invalid UUID", func(t *testing.T) {
		mockRepo := new(MockSessionRepository)
		handler := appsession.NewRevokeAllHandler(mockRepo)
		ctx := context.Background()

		cmd := appsession.RevokeAllCommand{UserID: "not-a-valid-uuid"}
		err := handler.Handle(ctx, cmd)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid user ID")
	})
}
