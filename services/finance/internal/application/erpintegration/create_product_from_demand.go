package erpintegration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Sentinel errors of CreateProductFromDemand (mapped by the delivery layer).
var (
	// ErrCoverageAlreadyCovered: the line is OK or already has a product (409).
	ErrCoverageAlreadyCovered = errors.New("coverage line is already covered")
	// ErrCoverageNotNoMapping: only NO_MAPPING lines can be created from (412).
	ErrCoverageNotNoMapping = errors.New("coverage line status is not NO_MAPPING")
	// ErrCoverageMBLine: MB lines are never auto-created (412; decision Q5/N-5).
	ErrCoverageMBLine = errors.New("masterbatch line: link the MB-head product")
	// ErrCoverageUnknownType: the item prefix matches no product type (412).
	ErrCoverageUnknownType = errors.New("no product type matches the ERP item code")
)

// NewDemandProduct is the product-master create input (AX product).
type NewDemandProduct struct {
	ProductTypeID int32
	ProductName   string
	ShadeCode     string
	ActorUserID   string
}

// DemandProductLink is the D-LINK input.
type DemandProductLink struct {
	ProductSysID int64
	ErpItemCode  string
	ErpShadeCode string
	ActorUserID  string
}

// ProductFromDemandCoverage is the coverage read port.
type ProductFromDemandCoverage interface {
	GetByID(ctx context.Context, batchID, cecID int64) (domain.CoverageLine, error)
}

// ProductFromDemandCreator creates a product master.
type ProductFromDemandCreator interface {
	CreateProduct(ctx context.Context, cmd NewDemandProduct) (int64, error)
}

// ProductFromDemandLinker links the new product to the ERP item (D-LINK).
type ProductFromDemandLinker interface {
	Link(ctx context.Context, cmd DemandProductLink) error
}

// ProductFromDemandTypes resolves a product type by code.
type ProductFromDemandTypes interface {
	GetByCode(ctx context.Context, code string) (*cptdomain.CostProductType, error)
}

// ProductFromDemandNames optionally resolves the ERP item name ("" = unknown).
type ProductFromDemandNames interface {
	ItemName(ctx context.Context, itemCode string) string
}

// CreateProductFromDemand creates a yarn product master from a NO_MAPPING
// coverage line and links it to the ERP item. PostgreSQL only; never Oracle.
type CreateProductFromDemand struct {
	cov    ProductFromDemandCoverage
	create ProductFromDemandCreator
	link   ProductFromDemandLinker
	types  ProductFromDemandTypes
	names  ProductFromDemandNames // may be nil
}

// NewCreateProductFromDemand constructs the use case; names may be nil.
func NewCreateProductFromDemand(cov ProductFromDemandCoverage, c ProductFromDemandCreator, l ProductFromDemandLinker, t ProductFromDemandTypes, names ProductFromDemandNames) *CreateProductFromDemand {
	return &CreateProductFromDemand{cov: cov, create: c, link: l, types: t, names: names}
}

// Handle runs the use case and returns the new product_sys_id.
func (u *CreateProductFromDemand) Handle(ctx context.Context, batchID, cecID int64, actor string) (int64, error) {
	line, err := u.cov.GetByID(ctx, batchID, cecID)
	if err != nil {
		return 0, err
	}
	if line.Status == domain.CoverageOK || line.ProductSysID != nil {
		return 0, ErrCoverageAlreadyCovered
	}
	if line.Status != domain.CoverageNoMapping {
		return 0, ErrCoverageNotNoMapping
	}
	if line.Kind == domain.ItemKindMB {
		return 0, ErrCoverageMBLine
	}
	code := strings.ToUpper(strings.TrimSpace(line.ItemCode))
	pt, err := u.resolveType(ctx, code)
	if err != nil {
		return 0, err
	}
	name := code
	if u.names != nil {
		if n := strings.TrimSpace(u.names.ItemName(ctx, code)); n != "" {
			name = n
		}
	}
	id, err := u.create.CreateProduct(ctx, NewDemandProduct{
		ProductTypeID: pt.TypeID(), ProductName: name, ShadeCode: line.ShadeCode, ActorUserID: actor,
	})
	if err != nil {
		return 0, err
	}
	if err := u.link.Link(ctx, DemandProductLink{
		ProductSysID: id, ErpItemCode: code, ErpShadeCode: line.ShadeCode, ActorUserID: actor,
	}); err != nil {
		return 0, fmt.Errorf("product %d created but ERP link failed: %w", id, err)
	}
	return id, nil
}

// resolveType matches the longest 3-letter type prefix (POY/PTY/ITY...) of
// the item code against the product-type codes.
func (u *CreateProductFromDemand) resolveType(ctx context.Context, code string) (*cptdomain.CostProductType, error) {
	if len(code) >= 3 {
		pt, err := u.types.GetByCode(ctx, code[:3])
		if err == nil {
			return pt, nil
		}
		if !errors.Is(err, cptdomain.ErrNotFound) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrCoverageUnknownType, code)
}
