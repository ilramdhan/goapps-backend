package mbhead_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func newShade(t *testing.T, seqNo int32, code, name string) *mbhead.Shade {
	t.Helper()
	s, err := mbhead.NewShade(uuid.New(), seqNo, code, name, "admin")
	require.NoError(t, err)
	return s
}

func TestNewShade_Success(t *testing.T) {
	parent := uuid.New()
	s, err := mbhead.NewShade(parent, 2, "SH-02", "Shade Two", "admin")
	require.NoError(t, err)
	assert.Equal(t, parent, s.MBHeadID())
	assert.Equal(t, int32(2), s.SeqNo())
	assert.Equal(t, "SH-02", s.ShadeCode())
	assert.Equal(t, "Shade Two", s.ShadeName())
	assert.False(t, s.IsDeleted())
}

func TestNewShade_Rejections(t *testing.T) {
	tests := []struct {
		name      string
		seqNo     int32
		code      string
		shadeName string
		createdBy string
		wantErr   error
	}{
		{"seq no too low", 1, "SH-02", "Shade Two", "admin", mbhead.ErrInvalidShadeSeqNo},
		{"seq no too high", 4, "SH-02", "Shade Two", "admin", mbhead.ErrInvalidShadeSeqNo},
		{"empty code", 2, "", "Shade Two", "admin", mbhead.ErrEmptyShadeCode},
		{"code too long", 2, strings.Repeat("a", 21), "Shade Two", "admin", mbhead.ErrShadeCodeTooLong},
		{"empty name", 2, "SH-02", "", "admin", mbhead.ErrEmptyShadeName},
		{"name too long", 2, "SH-02", strings.Repeat("a", 101), "admin", mbhead.ErrShadeNameTooLong},
		{"empty created_by", 2, "SH-02", "Shade Two", "", mbhead.ErrEmptyCreatedBy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mbhead.NewShade(uuid.New(), tt.seqNo, tt.code, tt.shadeName, tt.createdBy)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestShade_UpdateAndSoftDelete(t *testing.T) {
	s := newShade(t, 2, "SH-02", "Shade Two")
	require.NoError(t, s.Update("SH-02B", "Shade Two B", "editor"))
	assert.Equal(t, "SH-02B", s.ShadeCode())
	assert.Equal(t, "Shade Two B", s.ShadeName())
	assert.NotNil(t, s.UpdatedAt())

	assert.ErrorIs(t, s.Update("", "x", "editor"), mbhead.ErrEmptyShadeCode)

	require.NoError(t, s.SoftDelete("admin"))
	assert.True(t, s.IsDeleted())
	assert.ErrorIs(t, s.SoftDelete("admin"), mbhead.ErrAlreadyDeleted)
	assert.ErrorIs(t, s.Update("SH-03", "x", "editor"), mbhead.ErrAlreadyDeleted)
}

// TestReplaceShades_MaxThreeRule covers spec section 4.2: header shade + at most 2 children.
func TestReplaceShades_MaxThreeRule(t *testing.T) {
	tests := []struct {
		name    string
		shades  func(t *testing.T) []*mbhead.Shade
		wantErr error
	}{
		{
			name:   "none",
			shades: func(*testing.T) []*mbhead.Shade { return nil },
		},
		{
			name: "one child",
			shades: func(t *testing.T) []*mbhead.Shade {
				return []*mbhead.Shade{newShade(t, 2, "SH-02", "Two")}
			},
		},
		{
			name: "two children is the maximum",
			shades: func(t *testing.T) []*mbhead.Shade {
				return []*mbhead.Shade{newShade(t, 2, "SH-02", "Two"), newShade(t, 3, "SH-03", "Three")}
			},
		},
		{
			name: "three children exceeds the maximum",
			shades: func(t *testing.T) []*mbhead.Shade {
				return []*mbhead.Shade{
					newShade(t, 2, "SH-02", "Two"),
					newShade(t, 3, "SH-03", "Three"),
					newShade(t, 3, "SH-04", "Four"),
				}
			},
			wantErr: mbhead.ErrTooManyShades,
		},
		{
			name: "duplicate shade code among children",
			shades: func(t *testing.T) []*mbhead.Shade {
				return []*mbhead.Shade{newShade(t, 2, "SH-02", "Two"), newShade(t, 3, "SH-02", "Three")}
			},
			wantErr: mbhead.ErrDuplicateShadeCode,
		},
		{
			name: "child repeats the header shade code",
			shades: func(t *testing.T) []*mbhead.Shade {
				return []*mbhead.Shade{newShade(t, 2, "SH-01", "Header dup")}
			},
			wantErr: mbhead.ErrShadeCodeMatchesHeader,
		},
		{
			name: "duplicate sequence number",
			shades: func(t *testing.T) []*mbhead.Shade {
				return []*mbhead.Shade{newShade(t, 2, "SH-02", "Two"), newShade(t, 2, "SH-03", "Three")}
			},
			wantErr: mbhead.ErrDuplicateShadeSeqNo,
		},
		{
			name:    "nil element",
			shades:  func(*testing.T) []*mbhead.Shade { return []*mbhead.Shade{nil} },
			wantErr: mbhead.ErrShadeNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := mbhead.New(validInput()) // header shade code is "SH-01"
			require.NoError(t, err)
			err = e.ReplaceShades(tt.shades(t))
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			for _, s := range e.Shades() {
				assert.Equal(t, e.ID(), s.MBHeadID(), "parent must be re-pointed at the head")
			}
		})
	}
}

func TestReplaceShades_ClearsExisting(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)
	require.NoError(t, e.ReplaceShades([]*mbhead.Shade{newShade(t, 2, "SH-02", "Two")}))
	require.Len(t, e.Shades(), 1)

	require.NoError(t, e.ReplaceShades(nil))
	assert.Empty(t, e.Shades())
}

func TestAddShade(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)

	require.NoError(t, e.AddShade(newShade(t, 2, "SH-02", "Two")))
	require.NoError(t, e.AddShade(newShade(t, 3, "SH-03", "Three")))
	assert.Len(t, e.Shades(), 2)

	// A third child breaks the max-3 rule and must not mutate the existing set.
	assert.ErrorIs(t, e.AddShade(newShade(t, 3, "SH-04", "Four")), mbhead.ErrTooManyShades)
	assert.Len(t, e.Shades(), 2)

	assert.ErrorIs(t, e.AddShade(nil), mbhead.ErrShadeNotFound)
}

func TestSetShades_SkipsValidation(t *testing.T) {
	e, err := mbhead.New(validInput())
	require.NoError(t, err)
	// Legacy rows may violate current rules; hydration must still succeed.
	e.SetShades([]*mbhead.Shade{
		mbhead.ReconstructShade(uuid.New(), e.ID(), 9, "SH-01", "", time.Now(), "legacy", nil, nil, nil, nil),
	})
	assert.Len(t, e.Shades(), 1)
}
