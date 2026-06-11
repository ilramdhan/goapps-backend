package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitOTP_SixDigits(t *testing.T) {
	result := SplitOTP("840921")
	assert.Equal(t, []string{"8", "4", "0", "-", "9", "2", "1"}, result)
}

func TestSplitOTP_ShortInput(t *testing.T) {
	result := SplitOTP("123")
	assert.Equal(t, []string{"123"}, result)
}

func TestSplitOTP_EmptyInput(t *testing.T) {
	result := SplitOTP("")
	assert.Equal(t, []string{""}, result)
}

func TestSplitParagraphs_MultiParagraph(t *testing.T) {
	result := SplitParagraphs("First paragraph.\n\nSecond paragraph.")
	require.Len(t, result, 2)
	assert.Equal(t, "First paragraph.", result[0])
	assert.Equal(t, "Second paragraph.", result[1])
}

func TestSplitParagraphs_SingleNewlines(t *testing.T) {
	result := SplitParagraphs("Line one\nLine two")
	require.Len(t, result, 1)
	assert.Equal(t, "Line one Line two", result[0])
}

func TestSplitParagraphs_BlankLines(t *testing.T) {
	result := SplitParagraphs("\n\nFirst\n\n\n\nSecond\n\n")
	require.Len(t, result, 2)
	assert.Equal(t, "First", result[0])
	assert.Equal(t, "Second", result[1])
}

func TestNewRenderer_NonZeroYear(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost"})
	assert.NotZero(t, r.base.Year)
}

func TestRenderer_UnknownTemplate_ReturnsError(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost"})
	_, err := r.Render("nonexistent", nil)
	assert.Error(t, err)
}
