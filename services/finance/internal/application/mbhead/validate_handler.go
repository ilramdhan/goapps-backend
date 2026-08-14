// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
)

// Recipe parameter codes resolved from mst_mb_param at VALIDATE time and frozen onto the MB head.
const (
	paramCodeWaste             = "WASTE"
	paramCodeQualityLoss       = "QUALITY_LOSS"
	paramCodeEfficiency        = "EFFICIENCY"
	paramCodeDevExpense        = "DEV_EXPENSE"
	paramCodePacking           = "PACKING"
	paramCodeMBProdPerDay      = "MB_PROD_PER_DAY"
	paramCodeThroughputPerHour = "THROUGHPUT_PER_HOUR"
	paramCodeNoOfProcess       = "NO_OF_PROCESS"
)

// ValidateCommand represents the APPROVED → VALIDATED (or DRAFT → VALIDATED boughtout shortcut)
// transition command.
type ValidateCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
}

// ValidateHandler handles the ValidateMBHead command.
type ValidateHandler struct {
	repo      mbhead.Repository
	paramRepo mbparam.Repository
}

// NewValidateHandler creates a new ValidateHandler.
func NewValidateHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *ValidateHandler {
	return &ValidateHandler{repo: repo, paramRepo: paramRepo}
}

// Handle executes the validate MB Head transition. Own-production MBs must be APPROVED before
// validating; boughtout MBs skip straight from DRAFT (entity.Validate() enforces both paths —
// this handler only adds the extra own-production gate the domain state map alone can't express).
func (h *ValidateHandler) Handle(ctx context.Context, cmd ValidateCommand) (*mbhead.Entity, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, err
	}

	if !entity.IsBoughtout() && entity.EntryStatus() != mbhead.StatusApproved {
		return nil, mbhead.ErrInvalidTransition
	}

	params, err := h.resolveParamSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	applyHeadParamOverrides(params, entity)
	applyHeadNoOfProcess(params, entity)

	fromState := entity.EntryStatus()
	entity.FreezeParams(
		params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
		params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess,
	)
	if err := entity.Validate(); err != nil {
		return nil, err
	}

	if err := h.repo.TransitionWithAutoGen(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), "", cmd.ActorUserID, params, entity); err != nil {
		return nil, err
	}

	return entity, nil
}

// applyHeadParamOverrides lets the head's own per-product values win over the
// mst_mb_param master defaults resolved by resolveParamSnapshot.
//
// Throughput and number-of-process genuinely vary per MB (legacy throughput
// 20/34/40/55, no_of_process 1/2/3), and mb_prod_per_day can too. They are set
// on mst_mb_head, but freeze used to read only the param-master picklist
// defaults ('B'=40, 'D'=2) — so every MB froze the same values, MB_NET_PROD
// came out 40*0.94*16=601.6 across the board and conversion cost was flat.
// Worse, updateEntryStatusTx writes the snapshot back onto mst_mb_head, so the
// wrong defaults overwrote the correct per-head values on every Validate.
//
// The master default remains the fallback for a head that has no value of its
// own: freezing an empty string would make the NULLIF-to-numeric cast in
// updateEntryStatusTx store NULL, and would break the mst_mb_param_option
// numeric lookup during cost auto-gen.
//
// Waste / QualityLoss / Efficiency / DevExpense / Packing stay on the master
// defaults — legacy shows those are uniform. Extend this function the same way
// if any of them start varying per head.
func applyHeadParamOverrides(params *mbhead.ParamSnapshot, entity *mbhead.Entity) {
	if tp := entity.ParamThroughputPerHour(); tp != "" {
		params.ThroughputPerHour = tp
	}
	if np := entity.ParamNoOfProcess(); np != "" {
		params.NoOfProcess = np
	}
	if pd := entity.ParamMBProdPerDay(); pd != nil && *pd != "" {
		params.MBProdPerDay = pd
	}
}

// applyHeadNoOfProcess seeds the frozen no-of-process snapshot from the head's own
// user-selected mbh_no_of_process column (spec section 2.4).
//
// mbh_no_of_process and mbh_param_no_of_process are DISTINCT fields with distinct roles:
// the former is the user's editable choice (required on create/update since Phase 2), the
// latter is the immutable snapshot taken at Validate time. Before this column existed,
// Validate could only seed the snapshot from the mst_mb_param picklist default ('D'), which
// is why 4168 legacy rows all carry 'D' — a default, not a choice.
//
// This runs AFTER applyHeadParamOverrides so the user's current selection wins over a value
// frozen by an earlier Validate. It is deliberately NOT called from RefreezeHandler: re-freeze
// preserves what was frozen, and applyHeadParamOverrides alone still serves that path.
//
// An empty header value falls through to the applyHeadParamOverrides / master-default chain —
// freezing an empty string would break the mst_mb_param_option numeric lookup during auto-gen.
func applyHeadNoOfProcess(params *mbhead.ParamSnapshot, entity *mbhead.Entity) {
	if np := entity.NoOfProcess(); np != "" {
		params.NoOfProcess = np
	}
}

func (h *ValidateHandler) resolveParamSnapshot(ctx context.Context) (*mbhead.ParamSnapshot, error) {
	all, err := h.paramRepo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]*mbparam.Entity, len(all))
	for _, p := range all {
		byCode[p.Code()] = p
	}

	waste, err := resolveScalarParam(byCode, paramCodeWaste)
	if err != nil {
		return nil, err
	}
	qualityLoss, err := resolveScalarParam(byCode, paramCodeQualityLoss)
	if err != nil {
		return nil, err
	}
	efficiency, err := resolveScalarParam(byCode, paramCodeEfficiency)
	if err != nil {
		return nil, err
	}
	devExpense, err := resolveScalarParam(byCode, paramCodeDevExpense)
	if err != nil {
		return nil, err
	}
	packing, err := resolveScalarParam(byCode, paramCodePacking)
	if err != nil {
		return nil, err
	}
	mbProdPerDay, err := resolveScalarParam(byCode, paramCodeMBProdPerDay)
	if err != nil {
		return nil, err
	}
	throughputPerHour, err := resolvePicklistParam(byCode, paramCodeThroughputPerHour)
	if err != nil {
		return nil, err
	}
	noOfProcess, err := resolvePicklistParam(byCode, paramCodeNoOfProcess)
	if err != nil {
		return nil, err
	}

	return &mbhead.ParamSnapshot{
		Waste: waste, QualityLoss: qualityLoss, Efficiency: efficiency, DevExpense: devExpense,
		Packing: packing, MBProdPerDay: mbProdPerDay,
		ThroughputPerHour: throughputPerHour, NoOfProcess: noOfProcess,
	}, nil
}

func resolveScalarParam(byCode map[string]*mbparam.Entity, code string) (*string, error) {
	p, ok := byCode[code]
	if !ok {
		return nil, mbparam.ErrParamNotFound
	}
	v := p.DefaultValue()
	return &v, nil
}

func resolvePicklistParam(byCode map[string]*mbparam.Entity, code string) (string, error) {
	p, ok := byCode[code]
	if !ok {
		return "", mbparam.ErrParamNotFound
	}
	return p.DefaultOption(), nil
}
