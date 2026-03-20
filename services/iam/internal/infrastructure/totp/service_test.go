package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
)

func newTestService() *Service {
	return NewService(&config.TOTPConfig{
		Issuer: "GoAppsTest",
		Digits: 6,
		Period: 30,
	})
}

func TestGenerateSecret(t *testing.T) {
	svc := newTestService()

	secret, err := svc.GenerateSecret()
	require.NoError(t, err)
	assert.NotEmpty(t, secret)

	// Secret should be valid base32 (no padding).
	_, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	assert.NoError(t, err)
}

func TestGenerateSecret_UniquePerCall(t *testing.T) {
	svc := newTestService()

	s1, err := svc.GenerateSecret()
	require.NoError(t, err)

	s2, err := svc.GenerateSecret()
	require.NoError(t, err)

	assert.NotEqual(t, s1, s2)
}

func TestGenerateQRURI(t *testing.T) {
	svc := newTestService()

	uri := svc.GenerateQRURI("JBSWY3DPEHPK3PXP", "user@example.com")

	assert.True(t, strings.HasPrefix(uri, "otpauth://totp/"))
	assert.Contains(t, uri, "GoAppsTest")
	assert.Contains(t, uri, "user@example.com")
	assert.Contains(t, uri, "secret=JBSWY3DPEHPK3PXP")
	assert.Contains(t, uri, "digits=6")
	assert.Contains(t, uri, "period=30")
	assert.Contains(t, uri, "algorithm=SHA1")
}

func TestValidate_ValidCode(t *testing.T) {
	svc := newTestService()

	secret, err := svc.GenerateSecret()
	require.NoError(t, err)

	// Generate a valid code for the current period using the service's own logic.
	now := time.Now().Unix()
	currentPeriod := now / int64(svc.period)
	validCode := svc.generateCode(secret, currentPeriod)

	assert.True(t, svc.Validate(secret, validCode))
}

func TestValidate_InvalidCode(t *testing.T) {
	svc := newTestService()

	secret, err := svc.GenerateSecret()
	require.NoError(t, err)

	assert.False(t, svc.Validate(secret, "000000"))
	assert.False(t, svc.Validate(secret, "999999"))
	assert.False(t, svc.Validate(secret, "abcdef"))
}

func TestValidate_ClockSkew(t *testing.T) {
	svc := newTestService()

	secret, err := svc.GenerateSecret()
	require.NoError(t, err)

	now := time.Now().Unix()
	currentPeriod := now / int64(svc.period)

	// Code from previous period should still be accepted (clock skew tolerance).
	prevCode := svc.generateCode(secret, currentPeriod-1)
	assert.True(t, svc.Validate(secret, prevCode))

	// Code from next period should also be accepted.
	nextCode := svc.generateCode(secret, currentPeriod+1)
	assert.True(t, svc.Validate(secret, nextCode))
}

func TestValidate_ExpiredCode(t *testing.T) {
	svc := newTestService()

	secret, err := svc.GenerateSecret()
	require.NoError(t, err)

	now := time.Now().Unix()
	currentPeriod := now / int64(svc.period)

	// Code from 5 periods ago should be rejected (outside skew window).
	oldCode := svc.generateCode(secret, currentPeriod-5)
	assert.False(t, svc.Validate(secret, oldCode))
}

func TestValidate_EmptySecret(t *testing.T) {
	svc := newTestService()

	// Empty secret should not validate any code.
	assert.False(t, svc.Validate("", "123456"))
}

func TestValidate_EmptyCode(t *testing.T) {
	svc := newTestService()

	secret, err := svc.GenerateSecret()
	require.NoError(t, err)

	assert.False(t, svc.Validate(secret, ""))
}

func TestValidate_InvalidBase32Secret(t *testing.T) {
	svc := newTestService()

	// Invalid base32 secret should not match.
	assert.False(t, svc.Validate("!@#$%^&*()", "123456"))
}

func TestGenerateCode_Deterministic(t *testing.T) {
	svc := newTestService()

	secret := "JBSWY3DPEHPK3PXP"
	period := int64(12345678)

	code1 := svc.generateCode(secret, period)
	code2 := svc.generateCode(secret, period)

	assert.Equal(t, code1, code2)
	assert.Len(t, code1, svc.digits)
}

func TestGenerateCode_DifferentPeriodsDifferentCodes(t *testing.T) {
	svc := newTestService()

	secret := "JBSWY3DPEHPK3PXP"

	code1 := svc.generateCode(secret, 1000)
	code2 := svc.generateCode(secret, 1001)

	assert.NotEqual(t, code1, code2)
}

func TestGenerateCode_PaddedOutput(t *testing.T) {
	svc := newTestService()

	secret := "JBSWY3DPEHPK3PXP"

	// Run many periods; all codes should be zero-padded to the configured digit length.
	for i := int64(0); i < 100; i++ {
		code := svc.generateCode(secret, i)
		assert.Len(t, code, svc.digits, "period %d produced code with wrong length", i)
	}
}

func TestNewService(t *testing.T) {
	cfg := &config.TOTPConfig{
		Issuer: "TestIssuer",
		Digits: 8,
		Period: 60,
	}
	svc := NewService(cfg)

	assert.Equal(t, "TestIssuer", svc.issuer)
	assert.Equal(t, 8, svc.digits)
	assert.Equal(t, 60, svc.period)
}
