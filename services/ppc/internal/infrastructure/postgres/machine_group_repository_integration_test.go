// Gated by INTEGRATION_TEST=true; requires a reachable ppc_db (defaults match the
// local docker container goapps-ppc-postgres on port 5436).
package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinegroup"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestMachineGroupRepository_CRUD_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}

	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5436")
	user := getEnvOrDefault("TEST_DB_USER", "ppc")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "ppc123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "ppc_db")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	sqlDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, sqlDB.PingContext(ctx))

	db := postgres.NewDBFromSQL(sqlDB)
	svc := machinegroup.NewService(postgres.NewMachineGroupRepository(db))

	name := fmt.Sprintf("ITEST-%d", time.Now().UnixNano())

	// Create.
	created, err := svc.Create(ctx, machinegroup.CreateCommand{Name: name, Area: "TXT", CreatedBy: "itest"})
	require.NoError(t, err)
	require.NotZero(t, created.ID())
	t.Cleanup(func() {
		_, _ = sqlDB.ExecContext(context.Background(), "DELETE FROM machine_group WHERE group_id = $1", created.ID())
	})

	// Get.
	got, err := svc.Get(ctx, created.ID())
	require.NoError(t, err)
	require.Equal(t, name, got.Name())
	require.Equal(t, "TXT", got.Area().String())

	// Update.
	newName := name + "-U"
	spg := "SPG"
	updated, err := svc.Update(ctx, machinegroup.UpdateCommand{ID: created.ID(), Name: &newName, Area: &spg, UpdatedBy: "itest"})
	require.NoError(t, err)
	require.Equal(t, newName, updated.Name())
	require.Equal(t, "SPG", updated.Area().String())

	// List (search by the unique name).
	list, err := svc.List(ctx, machinegroup.ListQuery{Page: 1, PageSize: 10, Search: newName})
	require.NoError(t, err)
	require.GreaterOrEqual(t, list.TotalItems, int64(1))

	// Delete.
	require.NoError(t, svc.Delete(ctx, created.ID()))
	_, err = svc.Get(ctx, created.ID())
	require.Error(t, err)
}
