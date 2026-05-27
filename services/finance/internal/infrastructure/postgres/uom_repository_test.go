// Package postgres provides integration tests for the UOM repository.
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/uom"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// UOMRepositorySuite is the test suite for UOM repository.
type UOMRepositorySuite struct {
	suite.Suite
	db         *postgres.DB
	repo       uom.Repository
	ctx        context.Context
	categoryID uuid.UUID // Pre-seeded test category ID
}

func TestUOMRepositorySuite(t *testing.T) {
	// Skip if not in integration test mode
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(UOMRepositorySuite))
}

func (s *UOMRepositorySuite) SetupSuite() {
	s.ctx = context.Background()

	// Get connection details from environment or use defaults
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance_user")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance_pass")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db_test")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	// Connect to database
	db, err := sql.Open("postgres", dsn)
	require.NoError(s.T(), err)

	// Wait for DB to be ready
	err = waitForDB(db, 30*time.Second)
	require.NoError(s.T(), err)

	s.db = &postgres.DB{DB: db}
	s.repo = postgres.NewUOMRepository(s.db)

	// Setup test schema and seed category
	s.setupSchema()
	s.categoryID = s.seedTestCategory()
}

func (s *UOMRepositorySuite) TearDownSuite() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *UOMRepositorySuite) SetupTest() {
	// Clean up before each test
	_, _ = s.db.ExecContext(s.ctx, "DELETE FROM mst_uom WHERE uom_code LIKE 'TEST%'")
}

func (s *UOMRepositorySuite) setupSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS mst_uom_category (
		uom_category_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		category_code VARCHAR(50) NOT NULL UNIQUE,
		category_name VARCHAR(100) NOT NULL,
		description TEXT,
		is_active BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_by VARCHAR(100) NOT NULL,
		updated_at TIMESTAMP WITH TIME ZONE,
		updated_by VARCHAR(100),
		deleted_at TIMESTAMP WITH TIME ZONE,
		deleted_by VARCHAR(100)
	);
	CREATE TABLE IF NOT EXISTS mst_uom (
		uom_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		uom_code VARCHAR(20) NOT NULL UNIQUE,
		uom_name VARCHAR(100) NOT NULL,
		uom_category_id UUID NOT NULL REFERENCES mst_uom_category(uom_category_id),
		description TEXT,
		is_active BOOLEAN DEFAULT true,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		created_by VARCHAR(100) NOT NULL,
		updated_at TIMESTAMP WITH TIME ZONE,
		updated_by VARCHAR(100),
		deleted_at TIMESTAMP WITH TIME ZONE,
		deleted_by VARCHAR(100)
	);
	CREATE INDEX IF NOT EXISTS idx_mst_uom_code ON mst_uom(uom_code);
	CREATE INDEX IF NOT EXISTS idx_mst_uom_category_id ON mst_uom(uom_category_id);
	CREATE INDEX IF NOT EXISTS idx_mst_uom_active ON mst_uom(is_active) WHERE deleted_at IS NULL;
	`
	_, err := s.db.ExecContext(s.ctx, schema)
	require.NoError(s.T(), err)
}

func (s *UOMRepositorySuite) seedTestCategory() uuid.UUID {
	id := uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_uom_category (uom_category_id, category_code, category_name, created_by)
		VALUES ($1, 'TEST_WEIGHT', 'Test Weight', 'test')
		ON CONFLICT (category_code) DO UPDATE SET category_code = EXCLUDED.category_code
		RETURNING uom_category_id
	`, id)
	if err != nil {
		// If conflict, fetch existing
		_ = s.db.QueryRowContext(s.ctx,
			"SELECT uom_category_id FROM mst_uom_category WHERE category_code = 'TEST_WEIGHT'",
		).Scan(&id)
	}
	return id
}

func (s *UOMRepositorySuite) TestCreate() {
	code, _ := uom.NewCode("TEST_KG")
	entity, _ := uom.NewUOM(code, "Test Kilogram", s.categoryID, "Test description", "test_user")

	err := s.repo.Create(s.ctx, entity)
	assert.NoError(s.T(), err)

	// Verify created
	result, err := s.repo.GetByID(s.ctx, entity.ID())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "TEST_KG", result.Code().String())
	assert.Equal(s.T(), "Test Kilogram", result.Name())
}

func (s *UOMRepositorySuite) TestCreate_DuplicateCode() {
	code, _ := uom.NewCode("TEST_DUP")

	entity1, _ := uom.NewUOM(code, "First", s.categoryID, "", "test")
	entity2, _ := uom.NewUOM(code, "Second", s.categoryID, "", "test")

	err := s.repo.Create(s.ctx, entity1)
	assert.NoError(s.T(), err)

	err = s.repo.Create(s.ctx, entity2)
	assert.Error(s.T(), err)
}

func (s *UOMRepositorySuite) TestGetByID_NotFound() {
	result, err := s.repo.GetByID(s.ctx, uuid.New())
	assert.Error(s.T(), err)
	assert.Nil(s.T(), result)
}

func (s *UOMRepositorySuite) TestUpdate() {
	// Create first
	code, _ := uom.NewCode("TEST_UPD")
	entity, _ := uom.NewUOM(code, "Original", s.categoryID, "Old desc", "creator")
	_ = s.repo.Create(s.ctx, entity)

	// Update
	newName := "Updated Name"
	_ = entity.Update(&newName, nil, nil, nil, "updater")

	err := s.repo.Update(s.ctx, entity)
	assert.NoError(s.T(), err)

	// Verify
	result, _ := s.repo.GetByID(s.ctx, entity.ID())
	assert.Equal(s.T(), "Updated Name", result.Name())
}

func (s *UOMRepositorySuite) TestSoftDelete() {
	code, _ := uom.NewCode("TEST_DEL")
	entity, _ := uom.NewUOM(code, "To Delete", s.categoryID, "", "creator")
	_ = s.repo.Create(s.ctx, entity)

	err := s.repo.SoftDelete(s.ctx, entity.ID(), "deleter")
	assert.NoError(s.T(), err)

	// Should not be found
	result, err := s.repo.GetByID(s.ctx, entity.ID())
	assert.Error(s.T(), err)
	assert.Nil(s.T(), result)
}

func (s *UOMRepositorySuite) TestList() {
	// Create test data
	for i := 1; i <= 5; i++ {
		code, _ := uom.NewCode(fmt.Sprintf("TEST_LIST%d", i))
		entity, _ := uom.NewUOM(code, fmt.Sprintf("List Item %d", i), s.categoryID, "", "tester")
		_ = s.repo.Create(s.ctx, entity)
	}

	// List with pagination
	filter := uom.ListFilter{
		Search:   "TEST_LIST",
		Page:     1,
		PageSize: 3,
	}

	results, total, err := s.repo.List(s.ctx, filter)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), results, 3)
	assert.GreaterOrEqual(s.T(), total, int64(5))
}

func (s *UOMRepositorySuite) TestExistsByCode() {
	code, _ := uom.NewCode("TEST_EXISTS")
	entity, _ := uom.NewUOM(code, "Exists Test", s.categoryID, "", "tester")
	_ = s.repo.Create(s.ctx, entity)

	exists, err := s.repo.ExistsByCode(s.ctx, code)
	assert.NoError(s.T(), err)
	assert.True(s.T(), exists)

	nonExistCode, _ := uom.NewCode("NONEXIST")
	exists, err = s.repo.ExistsByCode(s.ctx, nonExistCode)
	assert.NoError(s.T(), err)
	assert.False(s.T(), exists)
}

// Helper functions

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func waitForDB(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := db.Ping(); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("database not ready within %v", timeout)
}
