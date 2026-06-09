package notification_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notifinfra "github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/notification"
)

func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("IAM_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://iam:iam123@localhost:5435/iam_db?sslmode=disable"
	}
	db, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDBUserResolver_GetByUserID_UnknownUUID_ReturnsEmpty(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}
	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)

	ids, err := r.GetByUserID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Empty(t, ids)
}

func TestDBUserResolver_GetByPermission_UnknownCode_ReturnsEmpty(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("set INTEGRATION_TEST=true to run")
	}
	db := testDB(t)
	r := notifinfra.NewDBUserResolver(db)

	ids, err := r.GetByPermission(context.Background(), "nonexistent.permission.code")
	require.NoError(t, err)
	assert.Empty(t, ids)
}
