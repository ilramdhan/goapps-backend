// Package costproductparameter wires CPP_ use cases.
package costproductparameter

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// Handlers is the bundled application layer.
type Handlers struct {
	repo cpp.Repository
}

// New wires the handlers.
func New(repo cpp.Repository) *Handlers {
	return &Handlers{repo: repo}
}

// ListProductRequiredParams returns the parameter form contents for a product.
func (h *Handlers) ListProductRequiredParams(ctx context.Context, productSysID int64, requiredOnly bool) ([]cpp.RequiredEntry, error) {
	exists, err := h.repo.ProductExists(ctx, productSysID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, cpp.ErrProductNotFound
	}
	return h.repo.ListForProduct(ctx, productSysID, requiredOnly)
}

// UpsertCommand bundles an upsert request.
type UpsertCommand struct {
	ProductSysID int64
	ParamID      uuid.UUID
	ValueNumeric *string
	ValueText    *string
	ValueFlag    *bool
	FilledBy     string
}

// Upsert validates against the param meta then writes via the repo.
func (h *Handlers) Upsert(ctx context.Context, cmd UpsertCommand) (*cpp.Value, error) {
	exists, err := h.repo.ProductExists(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, cpp.ErrProductNotFound
	}

	meta, err := h.repo.GetMeta(ctx, cmd.ParamID)
	if err != nil {
		return nil, err
	}
	if meta.IsPeriodDependent {
		return nil, cpp.ErrPeriodDependent
	}
	if err := cpp.EnsureValueShape(meta.DataType, cmd.ValueNumeric, cmd.ValueText, cmd.ValueFlag); err != nil {
		return nil, err
	}

	v := &cpp.Value{
		ProductSysID: cmd.ProductSysID,
		ParamID:      cmd.ParamID,
		ValueNumeric: cmd.ValueNumeric,
		ValueText:    cmd.ValueText,
		ValueFlag:    cmd.ValueFlag,
		FilledBy:     cmd.FilledBy,
		CreatedBy:    cmd.FilledBy,
	}
	if err := h.repo.Upsert(ctx, v); err != nil {
		return nil, fmt.Errorf("upsert cpp: %w", err)
	}
	return v, nil
}

// BatchResult summarizes a batch upsert.
type BatchResult struct {
	UpsertedCount    int32
	FailedCount      int32
	FailedParamCodes []string
}

// UpsertBatch runs Upsert for each command, capturing failures non-fatally.
func (h *Handlers) UpsertBatch(ctx context.Context, productSysID int64, cmds []UpsertCommand) (BatchResult, error) {
	exists, err := h.repo.ProductExists(ctx, productSysID)
	if err != nil {
		return BatchResult{}, err
	}
	if !exists {
		return BatchResult{}, cpp.ErrProductNotFound
	}

	var res BatchResult
	for _, cmd := range cmds {
		cmd.ProductSysID = productSysID
		if _, err := h.Upsert(ctx, cmd); err != nil {
			res.FailedCount++
			res.FailedParamCodes = append(res.FailedParamCodes, cmd.ParamID.String())
			continue
		}
		res.UpsertedCount++
	}
	return res, nil
}

// Delete clears a value.
func (h *Handlers) Delete(ctx context.Context, productSysID int64, paramID uuid.UUID) error {
	return h.repo.Delete(ctx, productSysID, paramID)
}

// =============================================================================
// CAPP_ Applicability use cases
// =============================================================================

// AddApplicable marks a param applicable to the product, defaulting is_required
// from the global mst_parameter flag if the caller didn't override.
func (h *Handlers) AddApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID, isRequired bool, displayOrder *int32, actor string) error {
	exists, err := h.repo.ProductExists(ctx, productSysID)
	if err != nil {
		return err
	}
	if !exists {
		return cpp.ErrProductNotFound
	}
	meta, err := h.repo.GetMeta(ctx, paramID)
	if err != nil {
		return err
	}
	if meta.IsPeriodDependent {
		return cpp.ErrPeriodDependent
	}

	a := &cpp.Applicability{
		ProductSysID: productSysID,
		ParamID:      paramID,
		IsRequired:   isRequired,
		DisplayOrder: displayOrder,
		CreatedBy:    actor,
	}
	return h.repo.AddApplicable(ctx, a)
}

// RemoveApplicable removes a param from a product (and its stored value).
func (h *Handlers) RemoveApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID) error {
	return h.repo.RemoveApplicable(ctx, productSysID, paramID)
}

// UpdateApplicable patches per-product override fields.
func (h *Handlers) UpdateApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID, isRequired *bool, displayOrder *int32, actor string) error {
	return h.repo.UpdateApplicable(ctx, productSysID, paramID, isRequired, displayOrder, actor)
}

// ListAvailable returns params NOT yet applicable to the product.
func (h *Handlers) ListAvailable(ctx context.Context, productSysID int64) ([]cpp.ParamMeta, error) {
	exists, err := h.repo.ProductExists(ctx, productSysID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, cpp.ErrProductNotFound
	}
	return h.repo.ListAvailableParams(ctx, productSysID)
}

// CheckMissing returns required-but-unbound param metas.
func (h *Handlers) CheckMissing(ctx context.Context, productSysID int64) ([]cpp.ParamMeta, error) {
	exists, err := h.repo.ProductExists(ctx, productSysID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, cpp.ErrProductNotFound
	}
	return h.repo.MissingRequired(ctx, productSysID)
}
