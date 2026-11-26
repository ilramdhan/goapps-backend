// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	appmbcomposition "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
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

// ValidateCommand represents the transition into VALIDATED — from SUBMITTED (the
// 2026-08-26 "Opsi A" Approve path), from APPROVED (legacy rows via the surviving
// ValidateMBHead RPC), or from DRAFT for the boughtout shortcut.
type ValidateCommand struct {
	MbhID       uuid.UUID
	ActorUserID string
	// ConfirmSupersede (proto ValidateMBHeadRequest.confirm_supersede) lets the first
	// Validate supersede live spins of other recipes with the same shade + FG. Without it
	// such a collision fails with mbspin.ErrSupersedeConfirmationRequired (409) and
	// nothing is written. Bulk Validate and Approve never set it.
	ConfirmSupersede bool
}

// ValidateHandler handles the ValidateMBHead command.
type ValidateHandler struct {
	repo      mbhead.Repository
	paramRepo mbparam.Repository
	// compositionRepo backs the [G.5] composition-sum gate. OPTIONAL: nil skips the
	// gate, keeping existing construction sites working unchanged.
	compositionRepo mbcomposition.Repository
	// lookupSync is the after-commit lookup-sync port; nil disables it.
	lookupSync LookupSyncTrigger
	// recompute refreshes cost snapshots after a re-validate; nil disables it.
	recompute RevalidateRecomputer
}

// RevalidateRecomputer recomputes, after commit and detached from the request, the MB
// cost snapshots of re-validated heads (and their MB dependents) for every unlocked
// period that already holds one. Implemented by mbbatch.TriggerHandler; must never block
// or fail the caller.
type RevalidateRecomputer interface {
	TriggerRevalidateRecompute(ctx context.Context, mbhIDs []string, actor string)
}

// WithRevalidateRecompute attaches the after-commit snapshot recompute. nil disables it.
func (h *ValidateHandler) WithRevalidateRecompute(r RevalidateRecomputer) *ValidateHandler {
	h.recompute = r
	return h
}

// WithLookupSyncTrigger attaches the after-commit lookup-sync port used to refresh
// yarn products repointed by a supersede (best-effort). nil disables it.
func (h *ValidateHandler) WithLookupSyncTrigger(t LookupSyncTrigger) *ValidateHandler {
	h.lookupSync = t
	return h
}

// triggerSupersedeSync queues, AFTER commit and detached from the request, a lookup sync
// for the NEW spin. The supersede repoint already moved every affected product's
// MB_SP_CODE pointer (and text) onto S_new, and the sync scope for a spin key is "products
// referencing that spin", so S_new alone reaches every repointed product; old spins no
// longer have referrers. Best-effort: failures are only logged.
func (h *ValidateHandler) triggerSupersedeSync(ctx context.Context, outcome *mbhead.SupersedeOutcome, actor string) {
	if h.lookupSync == nil || outcome == nil || outcome.NewSpinID == uuid.Nil || len(outcome.AffectedProductIDs) == 0 {
		return
	}
	trigger := h.lookupSync
	newSpin := outcome.NewSpinID
	logger := log.With().Str("mbs_id", newSpin.String()).Str("batch_id", outcome.BatchID.String()).Logger()
	bg := context.WithoutCancel(ctx)
	go func() {
		bgCtx, cancel := context.WithTimeout(bg, lookupSyncTriggerTimeout)
		defer cancel()
		if err := trigger.TriggerSpinSync(bgCtx, newSpin, actor); err != nil {
			logger.Warn().Err(err).Msg("MB supersede: lookup sync for new spin not queued")
		}
	}()
}

// NewValidateHandler creates a ValidateHandler without the [G.5] composition-sum
// gate. Retained so existing callers compile unchanged; prefer
// NewValidateHandlerWithComposition on the serving path.
func NewValidateHandler(repo mbhead.Repository, paramRepo mbparam.Repository) *ValidateHandler {
	return &ValidateHandler{repo: repo, paramRepo: paramRepo}
}

// NewValidateHandlerWithComposition creates a ValidateHandler that enforces the
// composition-sum rule before allowing the transition to VALIDATED (plan §11 item 78).
//
// This gate matters more than the submit one: VALIDATED freezes the composition into
// mst_mb_composition_version and triggers cost auto-gen, so a bad total validated here
// is copied into a permanent snapshot and costed. Still respects
// MB_COMPOSITION_SUM_ENFORCED.
func NewValidateHandlerWithComposition(
	repo mbhead.Repository, paramRepo mbparam.Repository, compositionRepo mbcomposition.Repository,
) *ValidateHandler {
	return &ValidateHandler{repo: repo, paramRepo: paramRepo, compositionRepo: compositionRepo}
}

// ownProductionValidateOrigins are the statuses an own-production MB may be validated
// FROM.
//
// 🔴 WIDENED 2026-08-26 (USER DECISION, "Opsi A"). The gate used to read
// ~~entity.EntryStatus() != mbhead.StatusApproved~~, i.e. APPROVED only. Under Opsi A the
// ApproveMBHead RPC drives THIS handler, so the recipe arrives here still SUBMITTED and
// the old gate would have refused every single approval.
//
// ⛔ StatusApproved is deliberately KEPT alongside it: production holds legacy rows parked
// in APPROVED, and the ValidateMBHead RPC (still alive — only its UI button was removed)
// is how they move on. Removing it would strand them.
var ownProductionValidateOrigins = map[string]struct{}{
	mbhead.StatusSubmitted: {},
	mbhead.StatusApproved:  {},
}

// boughtoutValidateOrigins are the statuses a BOUGHTOUT MB may be validated FROM.
//
// ⭐ **DIPERBARUI 2026-08-26** — sebelumnya boughtout diloloskan lewat bypass
// ~~`!entity.IsBoughtout()`~~ pada gate tunggal `validateOrigins`, yang membuat resep
// boughtout melewati gerbang aplikasi ini SEPENUHNYA untuk status APA PUN, dan pemeriksaan
// nyata baru terjadi satu lapis di bawah lewat entity.Validate() -> canTransition. Hasil
// akhirnya kebetulan tetap benar, tapi maksudnya tidak terbaca dari kode dan rawan lolos
// diam-diam kalau allowedTransitions berubah kelak. Sekarang asal boughtout dinyatakan
// EKSPLISIT di sini, sebagai gerbang setara (bukan bypass) dari gerbang own-production.
//
// Himpunan berikut dipilih supaya PERSIS mereproduksi perilaku lama (nol perubahan),
// dibuktikan lewat canTransition(from, StatusValidated) di state_machine.go:
//   - DRAFT: jalur pintas boughtout lama lewat RPC ValidateMBHead yang masih hidup.
//   - SUBMITTED: alur Opsi A sekarang (Submit -> Approve mendorong handler ini).
//   - APPROVED: baris legacy, sama seperti own-production.
//   - UNLOCK_REQUESTED: ⚠ INI BUKAN KEPUTUSAN DESAIN YANG DISENGAJA. Statusnya ada di
//     sini semata-mata karena allowedTransitions[StatusUnlockRequested] sudah memuat
//     StatusValidated (state_machine.go), dan gate lama membiarkan boughtout lewat tanpa
//     pengecekan origin sama sekali — jadi kombinasi ini SUDAH bisa berhasil hari ini.
//     Menghapusnya di sini akan MEMPERSEMPIT perilaku yang ada. Apakah kemampuan
//     "boughtout, sedang UNLOCK_REQUESTED, tetap bisa langsung divalidasi" ini memang
//     DIINGINKAN adalah pertanyaan produk terpisah yang belum diputuskan — jangan anggap
//     baris ini sebagai preseden desain, hanya sebagai potret perilaku yang dipertahankan.
var boughtoutValidateOrigins = map[string]struct{}{
	mbhead.StatusDraft:           {},
	mbhead.StatusSubmitted:       {},
	mbhead.StatusApproved:        {},
	mbhead.StatusUnlockRequested: {},
}

// Handle executes the validate MB Head transition.
//
// Own-production MBs must be SUBMITTED or APPROVED before validating; boughtout MBs may
// ~~additionally come straight from DRAFT~~.
//
// ⭐ **DIPERBARUI 2026-08-26** — boughtout's legal origins are now DRAFT, SUBMITTED,
// APPROVED, or UNLOCK_REQUESTED (see boughtoutValidateOrigins above for why each one is
// there, including the UNLOCK_REQUESTED caveat). entity.Validate() still enforces the
// underlying state-name transition — this handler adds the origin gate the domain state
// map alone can't express per recipe kind (design.md §2.1).
func (h *ValidateHandler) Handle(ctx context.Context, cmd ValidateCommand) (*mbhead.Entity, error) {
	entity, _, err := h.HandleWithOutcome(ctx, cmd)
	return entity, err
}

// HandleWithOutcome is Handle plus the supersede outcome (nil when nothing was superseded).
// Without cmd.ConfirmSupersede the repository is called exactly as before
// (TransitionWithAutoGen, which itself refuses a real shade+FG collision with
// mbspin.ErrSupersedeConfirmationRequired); with it, through TransitionWithAutoGenOpts.
func (h *ValidateHandler) HandleWithOutcome(ctx context.Context, cmd ValidateCommand) (*mbhead.Entity, *mbhead.SupersedeOutcome, error) {
	entity, err := h.repo.GetByID(ctx, cmd.MbhID)
	if err != nil {
		return nil, nil, err
	}

	origins := ownProductionValidateOrigins
	if entity.IsBoughtout() {
		origins = boughtoutValidateOrigins
	}
	if _, ok := origins[entity.EntryStatus()]; !ok {
		return nil, nil, mbhead.ErrInvalidTransition
	}

	// [G.5] Composition-sum gate: checked BEFORE params are frozen and before the
	// transition, so a broken composition is never snapshotted into
	// mst_mb_composition_version nor fed to cost auto-gen.
	if h.compositionRepo != nil {
		if err := appmbcomposition.EnforceHeadSum(ctx, h.compositionRepo, cmd.MbhID.String()); err != nil {
			return nil, nil, err
		}
	}

	params, err := h.resolveParamSnapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	applyHeadParamOverrides(params, entity)

	fromState := entity.EntryStatus()
	entity.FreezeParams(
		params.Waste, params.QualityLoss, params.Efficiency, params.DevExpense,
		params.Packing, params.MBProdPerDay, params.ThroughputPerHour, params.NoOfProcess,
	)
	if err := entity.Validate(); err != nil {
		return nil, nil, err
	}

	outcome, err := h.transition(ctx, cmd, entity, fromState, params)
	if err != nil {
		return nil, nil, err
	}
	h.triggerSupersedeSync(ctx, outcome, cmd.ActorUserID)
	// Only a re-validate (cost product already existed) can leave stale snapshots; a first
	// validate has none yet.
	if h.recompute != nil && entity.CostProductID() != 0 {
		h.recompute.TriggerRevalidateRecompute(ctx, []string{entity.ID().String()}, cmd.ActorUserID)
	}
	return entity, outcome, nil
}

// transition persists the validate. The unconfirmed path keeps calling
// TransitionWithAutoGen so existing callers/mocks see the exact same call.
func (h *ValidateHandler) transition(ctx context.Context, cmd ValidateCommand, entity *mbhead.Entity, fromState string, params *mbhead.ParamSnapshot) (*mbhead.SupersedeOutcome, error) {
	if !cmd.ConfirmSupersede {
		return nil, h.repo.TransitionWithAutoGen(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), "", cmd.ActorUserID, params, entity)
	}
	return h.repo.TransitionWithAutoGenOpts(ctx, entity.ID(), fromState, entity.EntryStatus(), entity.CurrentVersion(), "", cmd.ActorUserID, params, entity,
		mbhead.AutoGenOptions{ConfirmSupersede: true})
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

func (h *ValidateHandler) resolveParamSnapshot(ctx context.Context) (*mbhead.ParamSnapshot, error) {
	return resolveParamSnapshotFrom(ctx, h.paramRepo)
}

// resolveParamSnapshotFrom resolves the mst_mb_param master defaults into a snapshot.
// Shared by Validate, Refreeze and the Update cascade (via resolveRefreezeSnapshot).
func resolveParamSnapshotFrom(ctx context.Context, paramRepo mbparam.Repository) (*mbhead.ParamSnapshot, error) {
	all, err := paramRepo.ListActive(ctx)
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
