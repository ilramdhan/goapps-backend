package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// MergeCandidateLookup answers the WO merge-candidate query over the plan-item
// table. It lives in the postgres package (not the plan-item repository)
// because it is a work-order concern that happens to read plan-item rows.
type MergeCandidateLookup struct {
	db *DB
}

// NewMergeCandidateLookup creates a MergeCandidateLookup.
func NewMergeCandidateLookup(db *DB) *MergeCandidateLookup {
	return &MergeCandidateLookup{db: db}
}

// mergeSubjectColumns projects a plan-item row onto the merge predicate's
// inputs. Kept minimal: the candidate list is rendered from the full plan items
// fetched separately, so this query only decides membership.
const mergeSubjectColumns = `ppi_id, ppi_cpm_product_sys_id, ppi_machine_group_id,
	COALESCE(ppi_shade_code, ''), ppi_deadline, ppi_qty_target, ppi_status`

// naturalShadeList renders workorder.NaturalShades as a SQL IN list of bind
// placeholders starting at start, plus the matching args. Building it from the
// domain constant is the whole point: widening the natural set never needs a
// migration or an SQL edit.
func naturalShadeList(start int) (clause string, args []interface{}) {
	placeholders := make([]string, 0, len(workorder.NaturalShades))
	for i, s := range workorder.NaturalShades {
		placeholders = append(placeholders, fmt.Sprintf("$%d", start+i))
		args = append(args, s)
	}
	return "(" + strings.Join(placeholders, ", ") + ")", args
}

// statusList renders workorder.MergeableStatuses the same way.
func statusList(start int) (clause string, args []interface{}) {
	placeholders := make([]string, 0, len(workorder.MergeableStatuses))
	for i, s := range workorder.MergeableStatuses {
		placeholders = append(placeholders, fmt.Sprintf("$%d", start+i))
		args = append(args, s)
	}
	return "(" + strings.Join(placeholders, ", ") + ")", args
}

// Subject loads one plan item's merge projection.
func (l *MergeCandidateLookup) Subject(ctx context.Context, planItemID int64) (workorder.MergeSubject, error) {
	query := `SELECT ` + mergeSubjectColumns + ` FROM production_plan_item WHERE ppi_id = $1`
	var s workorder.MergeSubject
	err := l.db.QueryRowContext(ctx, query, planItemID).Scan(
		&s.PlanItemID, &s.ProductSysID, &s.MachineGroupID, &s.ShadeCode, &s.Deadline, &s.QtyTarget, &s.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workorder.MergeSubject{}, planitem.ErrNotFound
		}
		return workorder.MergeSubject{}, fmt.Errorf("failed to load merge subject: %w", err)
	}
	return s, nil
}

// Candidates returns the plan items mergeable with anchor, keyed on
// idx_ppi_merge_key (product, machine group, shade, deadline). Already-linked
// plan items are excluded, so a candidate can always be accepted.
func (l *MergeCandidateLookup) Candidates(
	ctx context.Context, anchor workorder.MergeSubject, windowDays int32,
) ([]int64, error) {
	// $1 product, $2 machine group, $3 anchor shade, $4 deadline, $5 window,
	// $6 anchor id, then the natural-shade set, then the status set.
	args := []interface{}{
		anchor.ProductSysID, anchor.MachineGroupID, anchor.ShadeCode,
		anchor.Deadline, windowDays, anchor.PlanItemID,
	}
	naturalClause, naturalArgs := naturalShadeList(len(args) + 1)
	args = append(args, naturalArgs...)
	statusClause, statusArgs := statusList(len(args) + 1)
	args = append(args, statusArgs...)

	query := `
		SELECT ppi.ppi_id
		FROM production_plan_item ppi
		WHERE ppi.ppi_cpm_product_sys_id = $1
		  AND ppi.ppi_machine_group_id   = $2
		  AND (UPPER(TRIM(COALESCE(ppi.ppi_shade_code, ''))) = UPPER(TRIM($3))
		       OR (UPPER(TRIM(COALESCE(ppi.ppi_shade_code, ''))) IN ` + naturalClause + `
		           AND UPPER(TRIM($3)) IN ` + naturalClause + `))
		  AND ABS(ppi.ppi_deadline - $4::DATE) <= $5
		  AND ppi.ppi_status IN ` + statusClause + `
		  AND ppi.ppi_id <> $6
		  AND NOT EXISTS (
		        SELECT 1 FROM wo_plan_item_link l WHERE l.wpl_plan_item_id = ppi.ppi_id)
		ORDER BY ppi.ppi_deadline, ppi.ppi_id`

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list merge candidates: %w", err)
	}
	defer closeRows(rows)

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan merge candidate: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate merge candidates: %w", err)
	}
	return ids, nil
}
