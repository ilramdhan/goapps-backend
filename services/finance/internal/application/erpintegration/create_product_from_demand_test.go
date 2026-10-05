package erpintegration

import (
	"context"
	"errors"
	"testing"

	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

type cpdCov struct{ line domain.CoverageLine }

func (c cpdCov) GetByID(context.Context, int64, int64) (domain.CoverageLine, error) {
	return c.line, nil
}

type cpdCreate struct{ created, linked int }

func (c *cpdCreate) CreateProduct(context.Context, NewDemandProduct) (int64, error) {
	c.created++
	return 77, nil
}
func (c *cpdCreate) Link(context.Context, DemandProductLink) error { c.linked++; return nil }

type cpdTypes struct{ known bool }

func (t cpdTypes) GetByCode(context.Context, string) (*cptdomain.CostProductType, error) {
	if !t.known {
		return nil, cptdomain.ErrNotFound
	}
	return &cptdomain.CostProductType{}, nil
}

func TestCreateProductFromDemand(t *testing.T) {
	one := int64(1)
	cases := []struct {
		name    string
		line    domain.CoverageLine
		known   bool
		wantErr error
		created int
	}{
		{"yarn happy", domain.CoverageLine{Kind: domain.ItemKindYarn, ItemCode: "POY100", Status: domain.CoverageNoMapping}, true, nil, 1},
		{"mb refused", domain.CoverageLine{Kind: domain.ItemKindMB, ItemCode: "CMB1", Status: domain.CoverageNoMapping}, true, ErrCoverageMBLine, 0},
		{"already covered", domain.CoverageLine{Kind: domain.ItemKindYarn, ItemCode: "POY100", Status: domain.CoverageOK, ProductSysID: &one}, true, ErrCoverageAlreadyCovered, 0},
		{"wrong status", domain.CoverageLine{Kind: domain.ItemKindYarn, ItemCode: "POY100", Status: domain.CoverageNoCost}, true, ErrCoverageNotNoMapping, 0},
		{"unknown type", domain.CoverageLine{Kind: domain.ItemKindYarn, ItemCode: "ZZZ100", Status: domain.CoverageNoMapping}, false, ErrCoverageUnknownType, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &cpdCreate{}
			u := NewCreateProductFromDemand(cpdCov{tc.line}, c, c, cpdTypes{tc.known}, nil)
			id, err := u.Handle(context.Background(), 1, 2, "u")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
			if c.created != tc.created || c.linked != tc.created {
				t.Fatalf("created=%d linked=%d", c.created, c.linked)
			}
			if tc.wantErr == nil && id != 77 {
				t.Fatalf("id=%d", id)
			}
		})
	}
}
