// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// mst_mb_head_shade column list, shared by every read site so the scan order cannot drift.
const shadeSelectCols = `
	SELECT mbhs_id, mbhs_mbh_id, mbhs_seq_no, mbhs_shade_code, mbhs_shade_name,
	       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
	FROM mst_mb_head_shade
`

// shadeQuerier is the narrow contract shared by *sql.DB and *sql.Tx, so the shade
// reconcile can run either on the pool or inside a caller-owned transaction.
type shadeQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ListShades retrieves the live additional shades for one MB head, ordered by seq no.
func (r *MBHeadRepository) ListShades(ctx context.Context, mbhID uuid.UUID) ([]*mbhead.Shade, error) {
	return listShades(ctx, r.db, mbhID)
}

// ListShadesByHeads retrieves the live additional shades for many MB heads in one query,
// keyed by head ID. The export calls this once instead of ListShades per row.
func (r *MBHeadRepository) ListShadesByHeads(
	ctx context.Context, mbhIDs []uuid.UUID,
) (map[uuid.UUID][]*mbhead.Shade, error) {
	out := make(map[uuid.UUID][]*mbhead.Shade, len(mbhIDs))
	if len(mbhIDs) == 0 {
		return out, nil
	}

	ids := make([]string, len(mbhIDs))
	for i, id := range mbhIDs {
		ids[i] = id.String()
	}

	rows, err := r.db.QueryContext(ctx,
		shadeSelectCols+`
		WHERE mbhs_mbh_id = ANY($1) AND deleted_at IS NULL
		ORDER BY mbhs_mbh_id, mbhs_seq_no ASC`,
		pq.Array(ids),
	)
	if err != nil {
		return nil, fmt.Errorf("list mb head shades by heads: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		s, scanErr := scanShade(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out[s.MBHeadID()] = append(out[s.MBHeadID()], s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mb head shades by heads: %w", err)
	}
	return out, nil
}

// ReplaceShades reconciles the child shade rows for one MB head to exactly the supplied set.
// An empty slice clears all children (spec section 4.4). All writes commit or roll back together.
func (r *MBHeadRepository) ReplaceShades(
	ctx context.Context, mbhID uuid.UUID, shades []*mbhead.Shade, actorUserID string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace shades tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Msg("rollback replace mb head shades")
		}
	}()

	if err := replaceShadesTx(ctx, tx, mbhID, shades, actorUserID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace shades: %w", err)
	}
	committed = true
	return nil
}

// replaceShadesTx performs the reconcile against any executor, so an enclosing
// transaction (e.g. an update handler saving header + shades atomically) can reuse it.
//
// Rows are matched on the (seq_no, shade_code) business key:
//   - exact key match, name differs  -> UPDATE the name in place (identity preserved)
//   - exact key match, name equal    -> untouched
//   - existing key not desired       -> soft-deleted
//   - desired key not existing       -> inserted
//
// All soft-deletes are issued before any insert. That ordering matters: uq_mbhs_code and
// uq_mbhs_seq are partial indexes over live rows only, so freeing a code or seq first lets a
// shade swap places (2:A/3:B -> 2:B/3:A) without tripping a unique violation mid-reconcile.
func replaceShadesTx(
	ctx context.Context, exec shadeQuerier, mbhID uuid.UUID, shades []*mbhead.Shade, actorUserID string,
) error {
	existing, err := listShades(ctx, exec, mbhID)
	if err != nil {
		return err
	}

	desired := make(map[string]*mbhead.Shade, len(shades))
	for _, s := range shades {
		desired[shadeKey(s.SeqNo(), s.ShadeCode())] = s
	}

	matched := make(map[string]*mbhead.Shade, len(existing))
	for _, e := range existing {
		key := shadeKey(e.SeqNo(), e.ShadeCode())
		want, ok := desired[key]
		if !ok {
			if delErr := softDeleteShade(ctx, exec, e.ID(), actorUserID); delErr != nil {
				return delErr
			}
			continue
		}
		matched[key] = e
		if want.ShadeName() != e.ShadeName() {
			if updErr := updateShadeName(ctx, exec, e.ID(), want.ShadeName(), actorUserID); updErr != nil {
				return updErr
			}
		}
	}

	for key, s := range desired {
		if _, ok := matched[key]; ok {
			continue
		}
		if insErr := insertShade(ctx, exec, mbhID, s, actorUserID); insErr != nil {
			return insErr
		}
	}
	return nil
}

func shadeKey(seqNo int32, code string) string {
	return fmt.Sprintf("%d|%s", seqNo, code)
}

func listShades(ctx context.Context, exec shadeQuerier, mbhID uuid.UUID) ([]*mbhead.Shade, error) {
	rows, err := exec.QueryContext(ctx,
		shadeSelectCols+` WHERE mbhs_mbh_id = $1 AND deleted_at IS NULL ORDER BY mbhs_seq_no ASC`,
		mbhID,
	)
	if err != nil {
		return nil, fmt.Errorf("list mb head shades: %w", err)
	}
	defer closeRows(rows)

	var items []*mbhead.Shade
	for rows.Next() {
		s, scanErr := scanShade(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mb head shades: %w", err)
	}
	return items, nil
}

func scanShade(rows *sql.Rows) (*mbhead.Shade, error) {
	var (
		id, mbhID            uuid.UUID
		seqNo                int32
		shadeCode, shadeName string
		createdAt            time.Time
		createdBy            string
		updatedAt, deletedAt sql.NullTime
		updatedBy, deletedBy sql.NullString
	)
	err := rows.Scan(
		&id, &mbhID, &seqNo, &shadeCode, &shadeName,
		&createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan mb head shade: %w", err)
	}
	return mbhead.ReconstructShade(
		id, mbhID, seqNo, shadeCode, shadeName,
		createdAt, createdBy,
		nullableTimePtr(updatedAt), nullableStringPtr(updatedBy),
		nullableTimePtr(deletedAt), nullableStringPtr(deletedBy),
	), nil
}

func insertShade(
	ctx context.Context, exec shadeQuerier, mbhID uuid.UUID, s *mbhead.Shade, actorUserID string,
) error {
	_, err := exec.ExecContext(ctx, `
		INSERT INTO mst_mb_head_shade (
			mbhs_id, mbhs_mbh_id, mbhs_seq_no, mbhs_shade_code, mbhs_shade_name,
			created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,NOW(),$6)
	`, s.ID(), mbhID, s.SeqNo(), s.ShadeCode(), s.ShadeName(), actorUserID)
	if err != nil {
		if dupErr := mbHeadUniqueViolation(err); dupErr != nil {
			return dupErr
		}
		return fmt.Errorf("insert mb head shade: %w", err)
	}
	return nil
}

func updateShadeName(
	ctx context.Context, exec shadeQuerier, id uuid.UUID, shadeName, actorUserID string,
) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE mst_mb_head_shade
		SET mbhs_shade_name = $2, updated_at = NOW(), updated_by = $3
		WHERE mbhs_id = $1 AND deleted_at IS NULL
	`, id, shadeName, actorUserID)
	if err != nil {
		return fmt.Errorf("update mb head shade: %w", err)
	}
	return nil
}

func softDeleteShade(ctx context.Context, exec shadeQuerier, id uuid.UUID, actorUserID string) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE mst_mb_head_shade
		SET deleted_at = NOW(), deleted_by = $2
		WHERE mbhs_id = $1 AND deleted_at IS NULL
	`, id, actorUserID)
	if err != nil {
		return fmt.Errorf("soft delete mb head shade: %w", err)
	}
	return nil
}
