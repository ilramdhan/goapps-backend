// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// DefaultMachineCode is the mst_machine.mc_code every MB Head is pinned to (B5).
//
// Melange batch heads always run on the "MB" machine, so the machine is derived,
// not chosen: whatever machine_id the client sends is IGNORED (plan §5 P5 K3).
const DefaultMachineCode = "MB"

// DefaultMBHStatus is the mbh_status written when the client sends none (B7).
//
// ⚠ Spelled "R and D" with spaces — not "RND", not "R&D". The value is compared
// literally against legacy Oracle rows.
const DefaultMBHStatus = "R and D"

// ErrDefaultMachineUnavailable is returned when the "MB" machine row cannot be
// resolved on create.
//
// 🔴 Deliberately an ERROR rather than a silent NULL machine_id (design §905):
// a head with no machine produces silently wrong costing, which is far worse
// than a create that fails loudly.
var ErrDefaultMachineUnavailable = errors.New("mbhead: default machine \"MB\" is not available")

// CreateCommand represents the create MB Head command.
type CreateCommand struct {
	MBCosting   string
	OracleSysID *string
	MgtName     *string
	Denier      *float64
	Filament    *int
	Dozing      *float64
	// ⛔ MBHCheckStatus removed (§11 item 106): the frozen Oracle trace is not a
	// writable command field on any path. The gRPC layer rejects requests carrying it.
	MBHStatus       *string
	MBHLdrPrsn      *float64
	MBHRunLdrPct    *float64
	MBHFinalProduct *string
	MBHCode         *string
	CreatedBy       string
	IsBoughtout     bool
	DevCode         string
	ShadeCode       string
	ShadeName       string
	CrossSection    string
	LustureCode     string

	// MachineID is accepted for wire compatibility but ⛔ NEVER used: B5 pins every
	// head to DefaultMachineCode. Kept as a field so the delivery layer does not have
	// to special-case the request, and so the ignoring is explicit rather than
	// accidental.
	MachineID *uuid.UUID

	// VSNumber and NoOfProcess are nil when the client omitted them, and nil is
	// persisted as NULL. ⛔ No default is substituted here — in particular ⛔ NOT
	// no_of_process = "D": that default (U-B) is still an OPEN user decision
	// (plan §11 item 70) and inventing it would write a wrong value onto every new
	// head. See the report accompanying this change.
	VSNumber    *string
	NoOfProcess *string

	// AdditionalShades carries the extra shade rows. U-C (2026-08-22): shades ARE
	// allowed at create time. Nil means the payload omitted the field, which is the
	// legacy shape and must succeed.
	AdditionalShades []mbhead.Shade
}

// CreateHandler handles the CreateMBHead command.
type CreateHandler struct {
	repo        mbhead.Repository
	machineRepo machine.Repository
}

// NewCreateHandler creates a new CreateHandler.
//
// machineRepo is required for B5 — the "MB" machine lookup.
func NewCreateHandler(repo mbhead.Repository, machineRepo machine.Repository) *CreateHandler {
	return &CreateHandler{repo: repo, machineRepo: machineRepo}
}

// Handle executes the create MB Head command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*mbhead.Entity, error) {
	exists, err := h.repo.ExistsByMBCosting(ctx, cmd.MBCosting)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, mbhead.ErrAlreadyExists
	}

	if err := h.assertVSNumberFree(ctx, cmd.VSNumber, uuid.Nil); err != nil {
		return nil, err
	}

	if err := h.assertDevCodeFree(ctx, cmd.DevCode, uuid.Nil); err != nil {
		return nil, err
	}

	machineID, err := h.resolveDefaultMachineID(ctx)
	if err != nil {
		return nil, err
	}

	entity, err := mbhead.New(mbhead.NewParams{
		MBCosting:       cmd.MBCosting,
		OracleSysID:     cmd.OracleSysID,
		MgtName:         cmd.MgtName,
		Denier:          cmd.Denier,
		Filament:        cmd.Filament,
		Dozing:          cmd.Dozing,
		MBHStatus:       defaultedStatus(cmd.MBHStatus),
		MBHLdrPrsn:      cmd.MBHLdrPrsn,
		MBHRunLdrPct:    cmd.MBHRunLdrPct,
		MBHFinalProduct: cmd.MBHFinalProduct,
		MBHCode:         cmd.MBHCode,
		CreatedBy:       cmd.CreatedBy,
		IsBoughtout:     cmd.IsBoughtout,
		DevCode:         cmd.DevCode,
		ShadeCode:       cmd.ShadeCode,
		ShadeName:       cmd.ShadeName,
		CrossSection:    cmd.CrossSection,
		LustureCode:     cmd.LustureCode,
		MachineID:       machineID,
		VSNumber:        cmd.VSNumber,
		NoOfProcess:     cmd.NoOfProcess,
	})
	if err != nil {
		return nil, err
	}

	// Validate the shade shape BEFORE the head row is written, so a bad shade list
	// cannot leave a half-created head behind.
	if cmd.AdditionalShades != nil {
		if err := entity.SetAdditionalShades(cmd.AdditionalShades); err != nil {
			return nil, err
		}
	}

	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}

	if len(entity.AdditionalShades()) > 0 {
		if err := h.repo.ReplaceShades(ctx, entity.ID(), entity.AdditionalShades(), cmd.CreatedBy); err != nil {
			return nil, err
		}
	}

	return entity, nil
}

// resolveDefaultMachineID looks up the "MB" machine (B5). A missing machine is an
// explicit failure, never a NULL machine_id.
func (h *CreateHandler) resolveDefaultMachineID(ctx context.Context) (*uuid.UUID, error) {
	if h.machineRepo == nil {
		return nil, ErrDefaultMachineUnavailable
	}
	m, err := h.machineRepo.GetByCode(ctx, DefaultMachineCode)
	if err != nil {
		if errors.Is(err, machine.ErrNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrDefaultMachineUnavailable, DefaultMachineCode)
		}
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("%w: %s", ErrDefaultMachineUnavailable, DefaultMachineCode)
	}
	id := m.ID()
	return &id, nil
}

// assertVSNumberFree enforces VS Number uniqueness in the application layer (B9).
//
// ⚠ The database index idx_mst_mb_head_vs_number is NON-UNIQUE on purpose: 177
// production heads share the value '0'. So the check must skip '0' and the empty
// string, or every new head would collide with that legacy block. No regex, no
// normalization, no trimming — the literal "NA" is a valid VS Number (J5).
func (h *CreateHandler) assertVSNumberFree(ctx context.Context, vsNumber *string, excludeID uuid.UUID) error {
	if !vsNumberNeedsUniquenessCheck(vsNumber) {
		return nil
	}
	taken, err := h.repo.ExistsByVSNumber(ctx, *vsNumber, excludeID)
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrDuplicateVSNumber
	}
	return nil
}

// assertDevCodeFree enforces Dev Code uniqueness in the application layer (U-D,
// 2026-08-22), mirroring assertVSNumberFree exactly.
//
// ⚠ mbh_dev_code has NO unique index and must not gain one: the user's rule is
// "unique for new data, leave existing legacy duplicates alone". A DB constraint
// would reject those rows outright. The empty string is exempt — DevCode is a
// plain string on the command, so "" means "the client sent no Dev Code" and
// probing it would make every head without one collide with the next.
func (h *CreateHandler) assertDevCodeFree(ctx context.Context, devCode string, excludeID uuid.UUID) error {
	if !devCodeNeedsUniquenessCheck(devCode) {
		return nil
	}
	taken, err := h.repo.ExistsByDevCode(ctx, devCode, excludeID)
	if err != nil {
		return err
	}
	if taken {
		return mbhead.ErrDuplicateDevCode
	}
	return nil
}

// devCodeNeedsUniquenessCheck reports whether a Dev Code value is subject to the
// uniqueness rule. Only "" is exempt — see assertDevCodeFree.
//
// ⚠ Unlike VS Number, no sentinel value such as "0" is exempted here: whether
// production holds a legacy block of placeholder Dev Codes has NOT been verified
// against the real data, and inventing an exemption would silently disable the
// rule for that value. See the report's user gate.
func devCodeNeedsUniquenessCheck(devCode string) bool {
	return devCode != ""
}

// vsNumberNeedsUniquenessCheck reports whether a VS Number value is subject to the
// uniqueness rule. Nil, "" and "0" are exempt — see assertVSNumberFree.
func vsNumberNeedsUniquenessCheck(vsNumber *string) bool {
	if vsNumber == nil {
		return false
	}
	return *vsNumber != "" && *vsNumber != "0"
}

// defaultedStatus applies the B7 default. An explicitly supplied status — including
// an empty one — is left exactly as sent.
func defaultedStatus(status *string) *string {
	if status != nil {
		return status
	}
	d := DefaultMBHStatus
	return &d
}
