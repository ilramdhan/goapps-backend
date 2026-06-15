package costroute_test

import (
	"errors"
	"testing"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

func TestValidateLevels_OK_SingleLevel(t *testing.T) {
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 100, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeItem, RmItemCode: "CHIPS_SD", RouteRmRatio: 1.0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestValidateLevels_OK_TwoLevels(t *testing.T) {
	// FG=100, Level-1 consumes intermediate 200 produced at level 2.
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 100, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeProduct, RmProductSysID: 200, RouteRmRatio: 1.0},
				},
			},
			{SeqID: 20, HeadID: 1, ProductSysID: 200, RouteLevel: 2, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeItem, RmItemCode: "CHIPS_SD", RouteRmRatio: 1.0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}

func TestValidateLevels_LevelOneMismatch(t *testing.T) {
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 999, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeItem, RmItemCode: "X", RouteRmRatio: 1.0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); !errors.Is(err, costroute.ErrLevelOneMismatch) {
		t.Fatalf("expected ErrLevelOneMismatch, got %v", err)
	}
}

func TestValidateLevels_UpstreamMissing(t *testing.T) {
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 100, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeProduct, RmProductSysID: 999, RouteRmRatio: 1.0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); !errors.Is(err, costroute.ErrUpstreamMissing) {
		t.Fatalf("expected ErrUpstreamMissing, got %v", err)
	}
}

func TestValidateLevels_UpstreamNotHigherLevel(t *testing.T) {
	// Level-2 seq references its own product (which is produced at level 2,
	// not strictly higher than 2) — invalid.
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 100, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeProduct, RmProductSysID: 200, RouteRmRatio: 1.0},
				},
			},
			{SeqID: 20, HeadID: 1, ProductSysID: 200, RouteLevel: 2, RouteSeq: 1,
				Rms: []*costroute.Rm{
					// Level-2 consuming a product also produced at level 2 (self / same level).
					{RmType: costroute.RmTypeProduct, RmProductSysID: 200, RouteRmRatio: 1.0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); !errors.Is(err, costroute.ErrUpstreamNotHigherLevel) {
		t.Fatalf("expected ErrUpstreamNotHigherLevel, got %v", err)
	}
}

func TestValidateLevels_NonPositiveRatio(t *testing.T) {
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 100, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeItem, RmItemCode: "X", RouteRmRatio: 0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); !errors.Is(err, costroute.ErrNonPositiveRatio) {
		t.Fatalf("expected ErrNonPositiveRatio, got %v", err)
	}
}

func TestValidateLevels_RmRefMismatch(t *testing.T) {
	// rm_type=PRODUCT but only item_code set.
	g := &costroute.Graph{
		Head: &costroute.Head{HeadID: 1, ProductSysID: 100},
		Seqs: []*costroute.Seq{
			{SeqID: 10, HeadID: 1, ProductSysID: 100, RouteLevel: 1, RouteSeq: 1,
				Rms: []*costroute.Rm{
					{RmType: costroute.RmTypeProduct, RmItemCode: "X", RouteRmRatio: 1.0},
				},
			},
		},
	}
	if err := g.ValidateLevels(); !errors.Is(err, costroute.ErrRmRefTypeMismatch) {
		t.Fatalf("expected ErrRmRefTypeMismatch, got %v", err)
	}
}

func TestHead_StatusTransitions(t *testing.T) {
	h := &costroute.Head{RoutingStatus: costroute.StatusDraft}
	if err := h.MarkComplete(); err != nil {
		t.Fatalf("draft->complete should succeed, got %v", err)
	}
	if h.RoutingStatus != costroute.StatusComplete {
		t.Fatalf("expected COMPLETE, got %s", h.RoutingStatus)
	}
	if err := h.Lock("test-actor"); err != nil {
		t.Fatalf("complete->lock should succeed, got %v", err)
	}
	if !h.IsLocked() {
		t.Fatalf("expected locked")
	}
	if h.LockedBy != "test-actor" {
		t.Fatalf("expected LockedBy=test-actor, got %s", h.LockedBy)
	}
	if err := h.MarkComplete(); !errors.Is(err, costroute.ErrInvalidStatusTransition) {
		t.Fatalf("locked->complete via MarkComplete should fail")
	}
	if err := h.Unlock("test-actor"); err != nil {
		t.Fatalf("locked->complete via Unlock should succeed, got %v", err)
	}
	if h.UnlockedBy != "test-actor" {
		t.Fatalf("expected UnlockedBy=test-actor, got %s", h.UnlockedBy)
	}
}
