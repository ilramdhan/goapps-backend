package postgres

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func TestMapBatchWriteError(t *testing.T) {
	for _, c := range []string{constraintCeibActive, constraintCeibInflight} {
		err := mapBatchWriteError("x", &pgconn.PgError{Code: sqlStateUniqueViolation, ConstraintName: c})
		assert.ErrorIs(t, err, erpintegration.ErrActiveBatchExists, c)
	}
	err := mapBatchWriteError("x", &pgconn.PgError{Code: sqlStateCheckViolation, ConstraintName: constraintCeibShadow})
	assert.ErrorIs(t, err, erpintegration.ErrShadowNotPushable)

	other := &pgconn.PgError{Code: sqlStateUniqueViolation, ConstraintName: "uk_ceib_period_seq"}
	err = mapBatchWriteError("x", other)
	assert.NotErrorIs(t, err, erpintegration.ErrActiveBatchExists)
	var pgErr *pgconn.PgError
	assert.True(t, errors.As(err, &pgErr))
	assert.ErrorIs(t, mapBatchWriteError("x", sql.ErrConnDone), sql.ErrConnDone)
}

func TestErpNullHelpers(t *testing.T) {
	assert.False(t, erpNullInt64(nil).Valid)
	v := int64(4)
	assert.Equal(t, sql.NullInt64{Int64: 4, Valid: true}, erpNullInt64(&v))
	assert.False(t, erpNullInt32(nil).Valid)
	w := int32(2)
	assert.Equal(t, sql.NullInt32{Int32: 2, Valid: true}, erpNullInt32(&w))

	assert.False(t, nullDecimalText(decimal.NullDecimal{}).Valid)
	d := nullDecimalText(decimal.NewNullDecimal(decimal.RequireFromString("1.50000")))
	assert.Equal(t, "1.5", d.String)
	got, err := parseNullDecimalText(sql.NullString{})
	require.NoError(t, err)
	assert.False(t, got.Valid)
	got, err = parseNullDecimalText(sql.NullString{String: "2.25", Valid: true})
	require.NoError(t, err)
	assert.True(t, got.Decimal.Equal(decimal.RequireFromString("2.25")))

	ts := time.Date(2026, 9, 1, 8, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	assert.Equal(t, "2026-09-01T01:00:00Z", loadedAtText(ts))
	assert.NotEmpty(t, loadedAtText(time.Time{}))

	assert.Equal(t, "{}", jsonOrEmptyObject(nil))
	assert.Equal(t, `{"a":1}`, jsonOrEmptyObject([]byte(`{"a":1}`)))
	assert.False(t, nullJSONArg(nil).Valid)
	assert.Nil(t, stampPtr(sql.NullTime{}, sql.NullString{}))
	at, by := stampArgs(nil)
	assert.False(t, at.Valid || by.Valid)
	assert.Nil(t, timePtr(sql.NullTime{}))
	assert.False(t, nullTimeArg(nil).Valid)
}

func TestReplaceGuardsRejectForeignLines(t *testing.T) {
	_, err := replaceDemand(t.Context(), nil, 0, nil)
	assert.Error(t, err)
	_, err = replaceDemand(t.Context(), nil, 1, []erpintegration.DemandLine{{BatchID: 2}})
	assert.Error(t, err)
	_, err = replaceCoverage(t.Context(), nil, 0, nil)
	assert.Error(t, err)
	_, err = replaceCoverage(t.Context(), nil, 1, []erpintegration.CoverageLine{{BatchID: 2}})
	assert.Error(t, err)
}
