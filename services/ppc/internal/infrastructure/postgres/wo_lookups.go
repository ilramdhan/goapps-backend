package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	lotdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lot"
	machinedomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/machine"
)

// MachineAreaLookup adapts the machine repository to the WO MachineLookup port.
type MachineAreaLookup struct {
	repo *MachineRepository
}

// NewMachineAreaLookup builds a machine-area lookup over the machine repository.
func NewMachineAreaLookup(repo *MachineRepository) *MachineAreaLookup {
	return &MachineAreaLookup{repo: repo}
}

// MachineArea returns the area code of a machine, or "" when not found.
func (l *MachineAreaLookup) MachineArea(ctx context.Context, machineID int64) (string, error) {
	m, err := l.repo.GetByID(ctx, machineID)
	if err != nil {
		if errors.Is(err, machinedomain.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return m.Area().String(), nil
}

// MachineNoLookup adapts the machine repository to the WO MachineNameLookup
// port, so a lot-generation failure can name the machine by its number rather
// than by an id the planner has never seen.
type MachineNoLookup struct {
	repo *MachineRepository
}

// NewMachineNoLookup builds a machine-number lookup over the machine repository.
func NewMachineNoLookup(repo *MachineRepository) *MachineNoLookup {
	return &MachineNoLookup{repo: repo}
}

// MachineNo returns the machine number of a machine, or "" when not found.
func (l *MachineNoLookup) MachineNo(ctx context.Context, machineID int64) (string, error) {
	m, err := l.repo.GetByID(ctx, machineID)
	if err != nil {
		if errors.Is(err, machinedomain.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return m.No(), nil
}

// LotExistsLookup adapts the lot repository to the WO LotLookup port.
type LotExistsLookup struct {
	repo *LotRepository
}

// NewLotExistsLookup builds a lot-existence lookup over the lot repository.
func NewLotExistsLookup(repo *LotRepository) *LotExistsLookup {
	return &LotExistsLookup{repo: repo}
}

// LotExists reports whether a lot number exists in lot_master.
func (l *LotExistsLookup) LotExists(ctx context.Context, lotNo string) (bool, error) {
	_, err := l.repo.GetByID(ctx, lotNo)
	if err != nil {
		if errors.Is(err, lotdomain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// PlanItemProductLookup resolves a plan item's finance product sys id for the WO
// parameter-resolution + snapshot flows.
type PlanItemProductLookup struct {
	db *DB
}

// NewPlanItemProductLookup builds a plan-item → product lookup over the DB.
func NewPlanItemProductLookup(db *DB) *PlanItemProductLookup {
	return &PlanItemProductLookup{db: db}
}

// ProductSysID returns the cpm_product_sys_id of a plan item, or 0 when absent.
func (l *PlanItemProductLookup) ProductSysID(ctx context.Context, planItemID int64) (int64, error) {
	var sysID int64
	err := l.db.QueryRowContext(ctx,
		`SELECT ppi_cpm_product_sys_id FROM production_plan_item WHERE ppi_id = $1`, planItemID).Scan(&sysID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to resolve plan item product: %w", err)
	}
	return sysID, nil
}
