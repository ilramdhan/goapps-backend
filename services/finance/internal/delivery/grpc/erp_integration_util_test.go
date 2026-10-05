package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErpUtil(t *testing.T) {
	assert.Equal(t, "-12", itoa(-12))
	assert.Equal(t, "B", trimPrefix("A_B", "A_"))
	assert.Equal(t, "B", trimPrefix("B", "A_"))
	assert.True(t, containsFold("POY-Item", "poy"))
	assert.False(t, containsFold("POY", "x"))

	var v struct{ A int }
	require.NoError(t, jsonUnmarshalLenient(nil, &v))
	require.NoError(t, jsonUnmarshalLenient([]byte(`{"A":3}`), &v))
	assert.Equal(t, 3, v.A)
	require.Error(t, jsonUnmarshalLenient([]byte(`{`), &v))
}
