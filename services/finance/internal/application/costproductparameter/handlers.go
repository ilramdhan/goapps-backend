// Package costproductparameter wires CPP_ use cases.
package costproductparameter

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// mbSpinLookupMasterCode is the mst_parameter.lookup_master_code value that
// marks a parameter as an MB_SPIN lookup, the trigger for resolving
// cpp_value_mb_spin_id in Upsert.
const mbSpinLookupMasterCode = "MB_SPIN"

// Handlers is the bundled application layer.
type Handlers struct {
	repo cpp.Repository
	// mbSpinRepo resolves cpp_value_mb_spin_id for MB_SPIN lookup parameters.
	// Nil-safe: when nil, Upsert skips resolution entirely and behaves exactly
	// as before this field existed (ValueMBSpinID stays nil, cpp_value_text is
	// still written as usual).
	mbSpinRepo mbspin.Repository
}

// New wires the handlers.
func New(repo cpp.Repository, mbSpinRepo mbspin.Repository) *Handlers {
	return &Handlers{repo: repo, mbSpinRepo: mbSpinRepo}
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
	entries, err := h.repo.ListForProduct(ctx, productSysID, requiredOnly)
	if err != nil {
		return nil, err
	}
	h.attachMBSpinCandidates(ctx, entries)
	return entries, nil
}

// attachMBSpinCandidates populates Value.MBSpinCandidates for rows that
// ListForProduct flagged as ambiguous (MBSpinCandidateCount != nil and > 1).
// Uses the same mbSpinRepo dependency as resolveMBSpinID, via
// ListByOrionItemCode — which shares the identical matching rule as the
// save-time resolver, so the candidate list shown never disagrees with what
// Upsert would actually resolve to.
func (h *Handlers) attachMBSpinCandidates(ctx context.Context, entries []cpp.RequiredEntry) {
	if h.mbSpinRepo == nil {
		return
	}
	for i := range entries {
		v := entries[i].Value
		if v == nil || v.MBSpinCandidateCount == nil || *v.MBSpinCandidateCount <= 1 {
			continue
		}
		if v.ValueText == nil || *v.ValueText == "" {
			continue
		}
		spins, err := h.mbSpinRepo.ListByOrionItemCode(ctx, *v.ValueText)
		if err != nil {
			continue // best-effort: leave candidates nil, count/badge still convey ambiguity
		}
		v.MBSpinCandidates = make([]cpp.MBSpinCandidateInfo, 0, len(spins))
		for _, s := range spins {
			v.MBSpinCandidates = append(v.MBSpinCandidates, cpp.MBSpinCandidateInfo{
				MBSID:         s.ID(),
				OrionItemCode: s.OrionItemCode(),
				MgtName:       s.MgtName(),
				Denier:        s.Denier(),
				Filament:      s.Filament(),
				MBSLdrPrsn:    s.MBSLdrPrsn(),
				MBSRunLdrPct:  s.MBSRunLdrPct(),
				MBSStatus:     s.MBSStatus(),
			})
		}
	}
}

// UpsertCommand bundles an upsert request.
type UpsertCommand struct {
	ProductSysID int64
	ParamID      uuid.UUID
	ValueNumeric *string
	ValueText    *string
	ValueFlag    *bool
	FilledBy     string
	// MBSpinIDOverride is the mst_mb_spin.mbs_id the user explicitly picked
	// from an MBSpinCandidate list for an ambiguous MB_SPIN lookup value. When
	// set, it is written directly to ValueMBSpinID and the normal
	// resolveMBSpinID ambiguity resolver is skipped entirely — the user's
	// explicit pick always wins over the ORION-code heuristic. ValueText is
	// still saved as-is alongside it (companion column).
	MBSpinIDOverride *uuid.UUID
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
	locked, err := h.repo.IsProductLocked(ctx, cmd.ProductSysID)
	if err != nil {
		return nil, err
	}
	if locked {
		return nil, cpp.ErrProductLocked
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
	switch {
	case cmd.MBSpinIDOverride != nil:
		// Explicit user pick from an MBSpinCandidate list always wins over the
		// ORION-code heuristic — completely bypasses resolveMBSpinID. Still
		// validated against mst_mb_spin so a stale/forged override can't write
		// a dangling FK.
		if h.mbSpinRepo == nil {
			return nil, cpp.ErrMBSpinOverrideNotFound
		}
		exists, existsErr := h.mbSpinRepo.ExistsByID(ctx, *cmd.MBSpinIDOverride)
		if existsErr != nil {
			return nil, fmt.Errorf("check mb_spin_id_override: %w", existsErr)
		}
		if !exists {
			return nil, cpp.ErrMBSpinOverrideNotFound
		}
		id := *cmd.MBSpinIDOverride
		v.ValueMBSpinID = &id
	case meta.LookupMasterCode == mbSpinLookupMasterCode:
		v.ValueMBSpinID = h.resolveMBSpinID(ctx, cmd.ValueText)
	}
	if err := h.repo.Upsert(ctx, v); err != nil {
		return nil, fmt.Errorf("upsert cpp: %w", err)
	}
	return v, nil
}

// resolveMBSpinID resolves an MB_SPIN lookup parameter's incoming text value to
// its permanent mst_mb_spin.mbs_id via resolveMBSpinValue, using this
// Handlers' configured mbSpinRepo.
func (h *Handlers) resolveMBSpinID(ctx context.Context, valueText *string) *uuid.UUID {
	return resolveMBSpinValue(ctx, h.mbSpinRepo, valueText)
}

// resolveMBSpinValue resolves an MB_SPIN lookup parameter's incoming text
// value to its permanent mst_mb_spin.mbs_id, for the companion
// cpp_value_mb_spin_id column. cpp_value_text keeps carrying the raw value
// unchanged regardless of what this returns — this is purely an additive
// companion resolution.
//
// Shared by both write paths that populate cpp_value_mb_spin_id: the
// interactive Upsert (Handlers.resolveMBSpinID) and the Excel bulk import
// (AsyncImportHandler.processBatch) — so both apply the identical
// ambiguity-safe rule.
//
// Resolution order: a valid UUID that matches exactly one spin wins outright
// (PK uniqueness — no ambiguity possible); otherwise an ORION item code that
// matches EXACTLY ONE spin wins. Any other case (empty value, unresolvable
// UUID, zero or multiple ORION code matches, or no resolver configured)
// returns nil rather than guessing — never an error, since the save must
// proceed via ValueText exactly as it did before this column existed.
func resolveMBSpinValue(ctx context.Context, repo mbspin.Repository, valueText *string) *uuid.UUID {
	if repo == nil || valueText == nil || *valueText == "" {
		return nil
	}
	raw := *valueText

	if parsed, parseErr := uuid.Parse(raw); parseErr == nil {
		if exists, err := repo.ExistsByID(ctx, parsed); err == nil && exists {
			return &parsed
		}
		return nil
	}

	id, ok, err := repo.ResolveUniqueByOrionItemCode(ctx, raw)
	if err != nil || !ok {
		return nil
	}
	return &id
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
	locked, err := h.repo.IsProductLocked(ctx, productSysID)
	if err != nil {
		return err
	}
	if locked {
		return cpp.ErrProductLocked
	}
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
	locked, err := h.repo.IsProductLocked(ctx, productSysID)
	if err != nil {
		return err
	}
	if locked {
		return cpp.ErrProductLocked
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
	locked, err := h.repo.IsProductLocked(ctx, productSysID)
	if err != nil {
		return err
	}
	if locked {
		return cpp.ErrProductLocked
	}
	return h.repo.RemoveApplicable(ctx, productSysID, paramID)
}

// UpdateApplicable patches per-product override fields.
func (h *Handlers) UpdateApplicable(ctx context.Context, productSysID int64, paramID uuid.UUID, isRequired *bool, displayOrder *int32, actor string) error {
	locked, err := h.repo.IsProductLocked(ctx, productSysID)
	if err != nil {
		return err
	}
	if locked {
		return cpp.ErrProductLocked
	}
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

// AddApplicableWithChildren adds a MASTER_LOOKUP or CALCULATED param and all its children
// (fill-group or formula inputs) atomically. fillGroupChildren should be the result of
// parameter.Repository.GetByFillGroup for MASTER_LOOKUP params, or the formula's InputParams
// for CALCULATED params; pass nil or empty slice otherwise. isRequired applies only to the
// trigger param.
func (h *Handlers) AddApplicableWithChildren(ctx context.Context, productSysID int64, triggerParamID uuid.UUID, isRequired bool, createdBy string, fillGroupChildren []uuid.UUID) error {
	exists, err := h.repo.ProductExists(ctx, productSysID)
	if err != nil {
		return err
	}
	if !exists {
		return cpp.ErrProductNotFound
	}
	locked, err := h.repo.IsProductLocked(ctx, productSysID)
	if err != nil {
		return err
	}
	if locked {
		return cpp.ErrProductLocked
	}
	return h.repo.AddApplicableWithChildren(ctx, productSysID, triggerParamID, isRequired, createdBy, fillGroupChildren)
}

// GetRemovePreview returns trigger + child param info for the confirm dialog.
func (h *Handlers) GetRemovePreview(ctx context.Context, productSysID int64, paramID uuid.UUID) (cpp.RemovePreview, error) {
	return h.repo.GetRemovePreview(ctx, productSysID, paramID)
}

// RemoveApplicableWithChildren removes a MASTER_LOOKUP param + all children + their CPP values atomically.
func (h *Handlers) RemoveApplicableWithChildren(ctx context.Context, productSysID int64, triggerParamID uuid.UUID, deletedBy string) error {
	locked, err := h.repo.IsProductLocked(ctx, productSysID)
	if err != nil {
		return err
	}
	if locked {
		return cpp.ErrProductLocked
	}
	return h.repo.RemoveApplicableWithChildren(ctx, productSysID, triggerParamID, deletedBy)
}
