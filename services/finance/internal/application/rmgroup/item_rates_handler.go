// Package rmgroup provides application layer handlers for RM group head and detail operations.
package rmgroup

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// GroupItemRates is one row per active detail of a group, joined with the
// period's `cst_item_cons_stk_po` row (LEFT JOIN — missing sync rows produce
// zero rates with an empty Period).
type GroupItemRates struct {
	ItemCode    string
	ItemName    string
	GradeCode   string
	ItemGrade   string
	UOMCode     string
	IsActive    bool
	IsDummy     bool
	Period      string
	ConsQty     float64
	ConsVal     float64
	ConsRate    float64
	StoresQty   float64
	StoresVal   float64
	StoresRate  float64
	DeptQty     float64
	DeptVal     float64
	DeptRate    float64
	LastPOQty1  float64
	LastPOVal1  float64
	LastPORate1 float64
	LastPOQty2  float64
	LastPOVal2  float64
	LastPORate2 float64
	LastPOQty3  float64
	LastPOVal3  float64
	LastPORate3 float64
}

// GroupItemRatesReader exposes the join used by the per-item rates view on the
// group detail page.
type GroupItemRatesReader interface {
	ListGroupItemRates(ctx context.Context, headID uuid.UUID, period string) ([]*GroupItemRates, error)
}

// GroupItemRatesQuery is the input for the item-rates query.
type GroupItemRatesQuery struct {
	HeadID string
	Period string
}

// GroupItemRatesHandler returns per-item stage rates for a group + period.
type GroupItemRatesHandler struct {
	reader GroupItemRatesReader
}

// NewGroupItemRatesHandler builds a GroupItemRatesHandler.
func NewGroupItemRatesHandler(reader GroupItemRatesReader) *GroupItemRatesHandler {
	return &GroupItemRatesHandler{reader: reader}
}

// Handle executes the query.
func (h *GroupItemRatesHandler) Handle(ctx context.Context, q GroupItemRatesQuery) ([]*GroupItemRates, error) {
	id, err := uuid.Parse(q.HeadID)
	if err != nil {
		return nil, rmgroup.ErrNotFound
	}
	rows, err := h.reader.ListGroupItemRates(ctx, id, q.Period)
	if err != nil {
		return nil, fmt.Errorf("list group item rates: %w", err)
	}
	return rows, nil
}
