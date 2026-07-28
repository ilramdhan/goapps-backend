package workorder_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	workorder "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

func TestShadesCompatible(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical", "RED", "RED", true},
		{"case and space folded", " red ", "RED", true},
		{"different colors", "RED", "BLU", false},
		{"both natural by code", "NL", "NATURAL", true},
		{"natural and blank", "NL", "", true},
		{"natural vs colored", "NL", "RED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, workorder.ShadesCompatible(tc.a, tc.b))
		})
	}
}

func TestIsNaturalShade(t *testing.T) {
	assert.True(t, workorder.IsNaturalShade("nl"))
	assert.True(t, workorder.IsNaturalShade(""))
	assert.False(t, workorder.IsNaturalShade("RED"))
}

func subject(id int64, shade string, day int) workorder.MergeSubject {
	return workorder.MergeSubject{
		PlanItemID:     id,
		ProductSysID:   900,
		MachineGroupID: 7,
		ShadeCode:      shade,
		Deadline:       time.Date(2026, 8, day, 0, 0, 0, 0, time.UTC),
		QtyTarget:      1000,
		Status:         "DRAFT",
	}
}

func TestCanMerge(t *testing.T) {
	anchor := subject(1, "NL", 20)

	t.Run("natural siblings within window", func(t *testing.T) {
		assert.True(t, workorder.CanMerge(anchor, subject(2, "NATURAL", 23), 0))
	})
	t.Run("itself is never a candidate", func(t *testing.T) {
		assert.False(t, workorder.CanMerge(anchor, subject(1, "NL", 20), 0))
	})
	t.Run("deadline outside default window", func(t *testing.T) {
		assert.False(t, workorder.CanMerge(anchor, subject(2, "NL", 30), 0))
	})
	t.Run("deadline inside widened window", func(t *testing.T) {
		assert.True(t, workorder.CanMerge(anchor, subject(2, "NL", 30), 30))
	})
	t.Run("different product", func(t *testing.T) {
		c := subject(2, "NL", 20)
		c.ProductSysID = 901
		assert.False(t, workorder.CanMerge(anchor, c, 0))
	})
	t.Run("different machine group", func(t *testing.T) {
		c := subject(2, "NL", 20)
		c.MachineGroupID = 8
		assert.False(t, workorder.CanMerge(anchor, c, 0))
	})
	t.Run("non-mergeable status", func(t *testing.T) {
		c := subject(2, "NL", 20)
		c.Status = "COMPLETED"
		assert.False(t, workorder.CanMerge(anchor, c, 0))
	})
	t.Run("confirmed status is mergeable", func(t *testing.T) {
		c := subject(2, "NL", 20)
		c.Status = "CONFIRMED"
		assert.True(t, workorder.CanMerge(anchor, c, 0))
	})
}
