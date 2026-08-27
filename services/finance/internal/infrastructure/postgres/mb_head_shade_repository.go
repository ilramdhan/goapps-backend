// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ListShades returns the additional shade rows for one MB head, ordered by
// sequence number, excluding soft-deleted rows.
//
// ⚠ Unrelated to MBCostERPRepository.ListShades, which reads cost_erp_shade — the
// two share a name and nothing else.
func (r *MBHeadRepository) ListShades(ctx context.Context, mbhID uuid.UUID) ([]mbhead.Shade, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT mbhs_seq_no, mbhs_shade_code, mbhs_shade_name
		FROM mst_mb_head_shade
		WHERE mbhs_mbh_id = $1 AND deleted_at IS NULL
		ORDER BY mbhs_seq_no
	`, mbhID)
	if err != nil {
		return nil, fmt.Errorf("list mb head shades: %w", err)
	}

	var items []mbhead.Shade
	for rows.Next() {
		var (
			seqNo int32
			code  string
			name  sql.NullString
		)
		if scanErr := rows.Scan(&seqNo, &code, &name); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan mb head shade: %w", scanErr)
		}
		// mbhs_shade_name is NULLABLE in 000483; NULL and '' both mean "no name".
		items = append(items, mbhead.Shade{SeqNo: seqNo, Code: code, Name: name.String})
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close mb head shade rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mb head shades: %w", err)
	}
	return items, nil
}

// ReplaceShades replaces a head's additional shade rows wholesale inside ONE
// transaction: every live row is soft-deleted, then the supplied rows are
// inserted. An empty slice therefore clears the head's additional shades.
//
// Soft-delete rather than DELETE keeps the audit trail; the partial unique index
// uix_mbhs_mbh_seq only covers rows WHERE deleted_at IS NULL, so a replacement row
// with the same sequence number does not collide with its retired predecessor.
//
// Shape validation (max 2 rows, seq 1..2, no duplicate seq, non-empty code) is the
// caller's responsibility via Entity.SetAdditionalShades — it is not repeated here.
func (r *MBHeadRepository) ReplaceShades(ctx context.Context, mbhID uuid.UUID, shades []mbhead.Shade, actorUserID string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			UPDATE mst_mb_head_shade
			SET deleted_at = NOW(), deleted_by = $2
			WHERE mbhs_mbh_id = $1 AND deleted_at IS NULL
		`, mbhID, actorUserID); err != nil {
			return fmt.Errorf("soft-delete mb head shades: %w", err)
		}
		for _, s := range shades {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO mst_mb_head_shade (
					mbhs_mbh_id, mbhs_seq_no, mbhs_shade_code, mbhs_shade_name,
					mbhs_created_by
				) VALUES ($1,$2,$3,NULLIF($4, ''),$5)
			`, mbhID, s.SeqNo, s.Code, s.Name, actorUserID); err != nil {
				return fmt.Errorf("insert mb head shade seq %d: %w", s.SeqNo, err)
			}
		}
		return nil
	})
}

// ExistsByVSNumber reports whether a live MB head other than excludeID already
// carries the given mbh_vs_number.
//
// ⚠ HELPER ONLY — idx_mst_mb_head_vs_number is deliberately NON-UNIQUE (000482):
// production holds 177 heads with '0'. The application layer decides when to
// consult this, and must exclude '0' and the empty string.
func (r *MBHeadRepository) ExistsByVSNumber(ctx context.Context, vsNumber string, excludeID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM mst_mb_head
			WHERE mbh_vs_number = $1 AND deleted_at IS NULL AND mbh_id <> $2
		)
	`, vsNumber, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check mb head vs number exists: %w", err)
	}
	return exists, nil
}

// ExistsByDevCode reports whether a live MB head other than excludeID already
// carries the given mbh_dev_code.
//
// ⚠ HELPER ONLY — there is deliberately NO unique index on mbh_dev_code (U-D):
// legacy duplicates must stay readable and editable. The application layer
// decides when to consult this, and must exclude the empty string.
func (r *MBHeadRepository) ExistsByDevCode(ctx context.Context, devCode string, excludeID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM mst_mb_head
			WHERE mbh_dev_code = $1 AND deleted_at IS NULL AND mbh_id <> $2
		)
	`, devCode, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check mb head dev code exists: %w", err)
	}
	return exists, nil
}
