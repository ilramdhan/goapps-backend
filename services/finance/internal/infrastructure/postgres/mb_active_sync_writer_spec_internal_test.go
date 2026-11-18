package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
)

func TestMBASSpecDiffers(t *testing.T) {
	den, fil := 380.0, 144
	denClose := 380.00001 // Fix4-equal
	other := 400.0
	in := mbactivesync.SyncSpinInput{TargetDen: &den, TargetFil: &fil, TargetCS: "TBL"}
	same := mbasSpinImage{Denier: &denClose, Filament: &fil, CrossSection: "tbl"}

	assert.False(t, mbasSpecDiffers(mbactivesync.SyncSpinInput{TargetCS: "RND"}, same), "no spec target: never (non-ROOT rows)")
	assert.False(t, mbasSpecDiffers(in, same), "fixed-point denier, case-insensitive cs")
	assert.True(t, mbasSpecDiffers(in, mbasSpinImage{Denier: &other, Filament: &fil, CrossSection: "TBL"}))
	assert.True(t, mbasSpecDiffers(in, mbasSpinImage{Filament: &fil, CrossSection: "TBL"}), "NULL denier")
	assert.True(t, mbasSpecDiffers(in, mbasSpinImage{Denier: &den, Filament: &fil}), "empty cross-section")
	noCS := in
	noCS.TargetCS = ""
	assert.False(t, mbasSpecDiffers(noCS, mbasSpinImage{Denier: &den, Filament: &fil, CrossSection: "RND"}), "empty target cs keeps current")
}

func TestMBASSpecEqIgnoresOldImages(t *testing.T) {
	a, b := 380.0, 400.0
	assert.True(t, mbasSpecEq(mbasSpinImage{Denier: &a}, mbasSpinImage{Denier: &b}), "images without has_spec")
	assert.False(t, mbasSpecEq(mbasSpinImage{Denier: &a, HasSpec: true}, mbasSpinImage{Denier: &b, HasSpec: true}))
}
