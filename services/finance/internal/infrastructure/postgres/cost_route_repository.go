package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"

	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// CostRouteRepository persists cost_route_head/_seq/_rm.
//
// S7.16a delivers only PromoteFromDraft + GetActiveByProduct -- enough to
// wire the routing draft Promote flow and replace the dropped
// cost_product_order repo. Full graph CRUD lands in S7.16b.
type CostRouteRepository struct {
	db *DB
}

// NewCostRouteRepository constructs a CostRouteRepository.
func NewCostRouteRepository(db *DB) *CostRouteRepository {
	return &CostRouteRepository{db: db}
}

var _ costroute.Repository = (*CostRouteRepository)(nil)

// PromoteFromDraft creates head + level-1 SEQ + RMs in a single transaction.
func (r *CostRouteRepository) PromoteFromDraft(ctx context.Context, in costroute.PromoteInput) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			// best-effort rollback; surfaced error from main path takes precedence
			_ = rbErr
		}
	}()

	var headID int64
	const insertHead = `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_version,
			crh_promoted_from_draft_id, crh_cyl_type_id,
			crh_created_by, crh_updated_by
		) VALUES ($1, 'DRAFT', 1, $2, $3, $4, $4)
		RETURNING crh_head_id`
	if err := tx.QueryRowContext(ctx, insertHead,
		in.ProductSysID, in.PromotedFromDraftID, in.CylTypeID, in.ActorUserID,
	).Scan(&headID); err != nil {
		if isRouteUniqueViolation(err) {
			return 0, costroute.ErrAlreadyExists
		}
		return 0, fmt.Errorf("insert route head: %w", err)
	}

	var seqID int64
	const insertSeq = `
		INSERT INTO cost_route_seq (
			crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
			crs_position_x, crs_position_y,
			crs_created_by, crs_updated_by
		) VALUES ($1, $2, 1, 1, 0, 0, $3, $3)
		RETURNING crs_seq_id`
	if err := tx.QueryRowContext(ctx, insertSeq,
		headID, in.ProductSysID, in.ActorUserID,
	).Scan(&seqID); err != nil {
		return 0, fmt.Errorf("insert level-1 seq: %w", err)
	}

	const insertRm = `
		INSERT INTO cost_route_rm (
			crm_seq_id, crm_parent_product_sys_id,
			crm_rm_product_sys_id, crm_rm_item_code, crm_rm_group_code,
			crm_rm_type, crm_route_rm_name, crm_route_rm_item_code,
			crm_route_rm_ratio, crm_sub_type, crm_notes,
			crm_created_by, crm_updated_by
		) VALUES ($1, $2, NULLIF($3, 0), NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12, $12)`
	for _, rm := range in.LevelOneRMs {
		if rm == nil {
			continue
		}
		ratio := rm.RouteRmRatio
		if ratio <= 0 {
			ratio = 1.0
		}
		if _, err := tx.ExecContext(ctx, insertRm,
			seqID, in.ProductSysID,
			rm.RmProductSysID, rm.RmItemCode, rm.RmGroupCode,
			rm.RmType, rm.RouteRmName, rm.RouteRmItemCode,
			ratio, rm.SubType, rm.Notes,
			in.ActorUserID,
		); err != nil {
			return 0, fmt.Errorf("insert route rm: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit promote tx: %w", err)
	}
	return headID, nil
}

// GetActiveByProduct returns the non-LOCKED head for the product or ErrNotFound.
func (r *CostRouteRepository) GetActiveByProduct(ctx context.Context, productSysID int64) (*costroute.Head, error) {
	const q = `
		SELECT crh_head_id, crh_product_sys_id, crh_routing_status, crh_version,
		       COALESCE(crh_promoted_from_draft_id, 0), COALESCE(crh_cyl_type_id, 0),
		       COALESCE(crh_notes, ''),
		       crh_created_at, crh_created_by, crh_updated_at, COALESCE(crh_updated_by, ''),
		       COALESCE(crh_locked_by, ''), crh_locked_at,
		       COALESCE(crh_unlocked_by, ''), crh_unlocked_at
		FROM cost_route_head
		WHERE crh_product_sys_id = $1
		  AND crh_deleted_at IS NULL
		  AND crh_routing_status <> 'LOCKED'
		LIMIT 1`
	h := &costroute.Head{}
	var lockedAt, unlockedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, q, productSysID).Scan(
		&h.HeadID, &h.ProductSysID, &h.RoutingStatus, &h.Version,
		&h.PromotedFromDraftID, &h.CylTypeID,
		&h.Notes,
		&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
		&h.LockedBy, &lockedAt,
		&h.UnlockedBy, &unlockedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costroute.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active route by product: %w", err)
	}
	if lockedAt.Valid {
		h.LockedAt = lockedAt.Time
	}
	if unlockedAt.Valid {
		h.UnlockedAt = unlockedAt.Time
	}
	return h, nil
}

func isRouteUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// =============================================================================
// GetHead / GetGraph
// =============================================================================

// GetHead returns the head row by id (or ErrNotFound).
func (r *CostRouteRepository) GetHead(ctx context.Context, headID int64) (*costroute.Head, error) {
	const q = `
		SELECT h.crh_head_id, h.crh_product_sys_id,
		       COALESCE(p.cpm_product_code, ''), COALESCE(p.cpm_product_name, ''),
		       h.crh_routing_status, h.crh_version,
		       COALESCE(h.crh_promoted_from_draft_id, 0), COALESCE(h.crh_cyl_type_id, 0),
		       COALESCE(h.crh_notes, ''),
		       h.crh_created_at, h.crh_created_by, h.crh_updated_at, COALESCE(h.crh_updated_by, ''),
		       COALESCE(h.crh_locked_by, ''), h.crh_locked_at,
		       COALESCE(h.crh_unlocked_by, ''), h.crh_unlocked_at
		FROM cost_route_head h
		LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.crh_product_sys_id
		WHERE h.crh_head_id = $1 AND h.crh_deleted_at IS NULL`
	h := &costroute.Head{}
	var lockedAt, unlockedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, q, headID).Scan(
		&h.HeadID, &h.ProductSysID, &h.ProductCode, &h.ProductName,
		&h.RoutingStatus, &h.Version,
		&h.PromotedFromDraftID, &h.CylTypeID, &h.Notes,
		&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
		&h.LockedBy, &lockedAt,
		&h.UnlockedBy, &unlockedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costroute.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get route head: %w", err)
	}
	if lockedAt.Valid {
		h.LockedAt = lockedAt.Time
	}
	if unlockedAt.Valid {
		h.UnlockedAt = unlockedAt.Time
	}
	return h, nil
}

// GetGraph returns the full graph for a head.
func (r *CostRouteRepository) GetGraph(ctx context.Context, headID int64) (*costroute.Graph, error) {
	head, err := r.GetHead(ctx, headID)
	if err != nil {
		return nil, err
	}
	seqs, err := r.loadSeqs(ctx, headID)
	if err != nil {
		return nil, err
	}
	rms, err := r.loadRms(ctx, headID)
	if err != nil {
		return nil, err
	}
	bySeq := make(map[int64][]*costroute.Rm, len(seqs))
	for _, rm := range rms {
		bySeq[rm.SeqID] = append(bySeq[rm.SeqID], rm)
	}
	for _, s := range seqs {
		s.Rms = bySeq[s.SeqID]
	}
	return &costroute.Graph{Head: head, Seqs: seqs}, nil
}

func (r *CostRouteRepository) loadSeqs(ctx context.Context, headID int64) ([]*costroute.Seq, error) {
	const q = `
		SELECT s.crs_seq_id, s.crs_head_id, s.crs_product_sys_id,
		       COALESCE(p.cpm_product_code, ''), COALESCE(p.cpm_product_name, ''),
		       s.crs_route_level, s.crs_route_seq,
		       COALESCE(s.crs_route_name, ''), COALESCE(s.crs_route_item_code, ''),
		       COALESCE(s.crs_route_shade_code, ''), COALESCE(s.crs_route_shade_name, ''),
		       s.crs_position_x, s.crs_position_y
		FROM cost_route_seq s
		LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = s.crs_product_sys_id
		WHERE s.crs_head_id = $1 AND s.crs_deleted_at IS NULL
		ORDER BY s.crs_route_level, s.crs_route_seq`
	rows, err := r.db.QueryContext(ctx, q, headID)
	if err != nil {
		return nil, fmt.Errorf("load route seqs: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costroute.Seq{}
	for rows.Next() {
		s := &costroute.Seq{}
		if err := rows.Scan(&s.SeqID, &s.HeadID, &s.ProductSysID, &s.ProductCode, &s.ProductName,
			&s.RouteLevel, &s.RouteSeq, &s.RouteName, &s.RouteItemCode, &s.RouteShadeCode, &s.RouteShadeName,
			&s.PositionX, &s.PositionY,
		); err != nil {
			return nil, fmt.Errorf("scan route seq: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route seqs: %w", err)
	}
	return out, nil
}

func (r *CostRouteRepository) loadRms(ctx context.Context, headID int64) ([]*costroute.Rm, error) {
	const q = `
		SELECT rm.crm_rm_id, rm.crm_seq_id, rm.crm_parent_product_sys_id, rm.crm_rm_type,
		       COALESCE(rm.crm_rm_product_sys_id, 0), COALESCE(rm.crm_rm_item_code, ''), COALESCE(rm.crm_rm_group_code, ''),
		       COALESCE(rm.crm_route_rm_name, ''), COALESCE(rm.crm_route_rm_item_code, ''),
		       COALESCE(rm.crm_route_rm_shade_code, ''), COALESCE(rm.crm_route_rm_shade_name, ''),
		       rm.crm_route_rm_ratio, COALESCE(rm.crm_uom_id, 0), COALESCE(rm.crm_sub_type, ''), COALESCE(rm.crm_notes, '')
		FROM cost_route_rm rm
		JOIN cost_route_seq s ON s.crs_seq_id = rm.crm_seq_id
		WHERE s.crs_head_id = $1
		ORDER BY rm.crm_seq_id, rm.crm_rm_id`
	rows, err := r.db.QueryContext(ctx, q, headID)
	if err != nil {
		return nil, fmt.Errorf("load route rms: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costroute.Rm{}
	for rows.Next() {
		rm := &costroute.Rm{}
		if err := rows.Scan(&rm.RmID, &rm.SeqID, &rm.ParentProductSysID, &rm.RmType,
			&rm.RmProductSysID, &rm.RmItemCode, &rm.RmGroupCode,
			&rm.RouteRmName, &rm.RouteRmItemCode, &rm.RouteRmShadeCode, &rm.RouteRmShadeName,
			&rm.RouteRmRatio, &rm.UomID, &rm.SubType, &rm.Notes,
		); err != nil {
			return nil, fmt.Errorf("scan route rm: %w", err)
		}
		out = append(out, rm)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate route rms: %w", err)
	}
	return out, nil
}

// =============================================================================
// SaveGraph (bulk diff + upsert in tx)
// =============================================================================

// SaveGraph diffs incoming seqs/rms against persisted state. Caller MUST have
// already passed Graph.ValidateLevels(); this method does not re-validate but
// trusts the input.
func (r *CostRouteRepository) SaveGraph(ctx context.Context, headID int64, in *costroute.Graph, actor string) (*costroute.Graph, error) { //nolint:gocognit,gocyclo // cohesive transactional DAG persistence
	if in == nil {
		return nil, fmt.Errorf("save route graph: nil graph")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	// 1. Build "keep" sets from the incoming payload.
	keepSeq := make(map[int64]struct{}, len(in.Seqs))
	for _, s := range in.Seqs {
		if s != nil && s.SeqID > 0 {
			keepSeq[s.SeqID] = struct{}{}
		}
	}
	// 2. Delete persisted seqs not in keep set (cascades RMs).
	rowsToDelete, err := tx.QueryContext(ctx, `SELECT crs_seq_id FROM cost_route_seq WHERE crs_head_id = $1`, headID)
	if err != nil {
		return nil, fmt.Errorf("list persisted seqs: %w", err)
	}
	deleteSeqs := []int64{}
	for rowsToDelete.Next() {
		var id int64
		if err := rowsToDelete.Scan(&id); err != nil {
			if cerr := rowsToDelete.Close(); cerr != nil {
				_ = cerr
			}
			return nil, fmt.Errorf("scan persisted seq id: %w", err)
		}
		if _, kept := keepSeq[id]; !kept {
			deleteSeqs = append(deleteSeqs, id)
		}
	}
	if err := rowsToDelete.Close(); err != nil {
		return nil, fmt.Errorf("close persisted seqs cursor: %w", err)
	}
	for _, id := range deleteSeqs {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cost_route_seq WHERE crs_seq_id = $1`, id); err != nil {
			return nil, fmt.Errorf("delete obsolete seq %d: %w", id, err)
		}
	}

	// 3. Upsert seqs (insert if seq_id=0, update otherwise).
	for _, s := range in.Seqs {
		if s == nil {
			continue
		}
		s.HeadID = headID
		if s.SeqID == 0 {
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO cost_route_seq (
					crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
					crs_route_name, crs_route_item_code, crs_route_shade_code, crs_route_shade_name,
					crs_position_x, crs_position_y,
					crs_created_by, crs_updated_by
				) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10,$11,$11)
				RETURNING crs_seq_id`,
				headID, s.ProductSysID, s.RouteLevel, s.RouteSeq,
				s.RouteName, s.RouteItemCode, s.RouteShadeCode, s.RouteShadeName,
				s.PositionX, s.PositionY,
				actor,
			).Scan(&s.SeqID); err != nil {
				return nil, fmt.Errorf("insert seq L%d/%d: %w", s.RouteLevel, s.RouteSeq, err)
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				UPDATE cost_route_seq SET
					crs_product_sys_id=$2, crs_route_level=$3, crs_route_seq=$4,
					crs_route_name=NULLIF($5,''), crs_route_item_code=NULLIF($6,''),
					crs_route_shade_code=NULLIF($7,''), crs_route_shade_name=NULLIF($8,''),
					crs_position_x=$9, crs_position_y=$10,
					crs_updated_at=now(), crs_updated_by=$11
				WHERE crs_seq_id=$1`,
				s.SeqID, s.ProductSysID, s.RouteLevel, s.RouteSeq,
				s.RouteName, s.RouteItemCode, s.RouteShadeCode, s.RouteShadeName,
				s.PositionX, s.PositionY,
				actor,
			); err != nil {
				return nil, fmt.Errorf("update seq %d: %w", s.SeqID, err)
			}
		}
	}

	// 4. Per-seq RM diff+upsert.
	for _, s := range in.Seqs {
		if s == nil {
			continue
		}
		keepRm := make(map[int64]struct{}, len(s.Rms))
		for _, rm := range s.Rms {
			if rm != nil && rm.RmID > 0 {
				keepRm[rm.RmID] = struct{}{}
			}
		}
		rowsR, err := tx.QueryContext(ctx, `SELECT crm_rm_id FROM cost_route_rm WHERE crm_seq_id=$1`, s.SeqID)
		if err != nil {
			return nil, fmt.Errorf("list persisted rms for seq %d: %w", s.SeqID, err)
		}
		deleteRms := []int64{}
		for rowsR.Next() {
			var id int64
			if err := rowsR.Scan(&id); err != nil {
				if cerr := rowsR.Close(); cerr != nil {
					_ = cerr
				}
				return nil, fmt.Errorf("scan persisted rm id: %w", err)
			}
			if _, kept := keepRm[id]; !kept {
				deleteRms = append(deleteRms, id)
			}
		}
		if err := rowsR.Close(); err != nil {
			return nil, fmt.Errorf("close persisted rms cursor: %w", err)
		}
		for _, id := range deleteRms {
			if _, err := tx.ExecContext(ctx, `DELETE FROM cost_route_rm WHERE crm_rm_id=$1`, id); err != nil {
				return nil, fmt.Errorf("delete obsolete rm %d: %w", id, err)
			}
		}
		for _, rm := range s.Rms {
			if rm == nil {
				continue
			}
			rm.SeqID = s.SeqID
			rm.ParentProductSysID = s.ProductSysID
			if rm.RmID == 0 {
				if err := tx.QueryRowContext(ctx, `
					INSERT INTO cost_route_rm (
						crm_seq_id, crm_parent_product_sys_id,
						crm_rm_product_sys_id, crm_rm_item_code, crm_rm_group_code,
						crm_rm_type,
						crm_route_rm_name, crm_route_rm_item_code, crm_route_rm_shade_code, crm_route_rm_shade_name,
						crm_route_rm_ratio, crm_uom_id, crm_sub_type, crm_notes,
						crm_created_by, crm_updated_by
					) VALUES ($1,$2,NULLIF($3,0),NULLIF($4,''),NULLIF($5,''),$6,
					          NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),
					          $11,NULLIF($12,0),NULLIF($13,''),NULLIF($14,''),$15,$15)
					RETURNING crm_rm_id`,
					s.SeqID, s.ProductSysID,
					rm.RmProductSysID, rm.RmItemCode, rm.RmGroupCode,
					rm.RmType,
					rm.RouteRmName, rm.RouteRmItemCode, rm.RouteRmShadeCode, rm.RouteRmShadeName,
					rm.RouteRmRatio, rm.UomID, rm.SubType, rm.Notes,
					actor,
				).Scan(&rm.RmID); err != nil {
					return nil, fmt.Errorf("insert rm under seq %d: %w", s.SeqID, err)
				}
			} else {
				if _, err := tx.ExecContext(ctx, `
					UPDATE cost_route_rm SET
						crm_rm_product_sys_id=NULLIF($2,0), crm_rm_item_code=NULLIF($3,''), crm_rm_group_code=NULLIF($4,''),
						crm_rm_type=$5,
						crm_route_rm_name=NULLIF($6,''), crm_route_rm_item_code=NULLIF($7,''),
						crm_route_rm_shade_code=NULLIF($8,''), crm_route_rm_shade_name=NULLIF($9,''),
						crm_route_rm_ratio=$10, crm_uom_id=NULLIF($11,0), crm_sub_type=NULLIF($12,''), crm_notes=NULLIF($13,''),
						crm_updated_at=now(), crm_updated_by=$14
					WHERE crm_rm_id=$1`,
					rm.RmID,
					rm.RmProductSysID, rm.RmItemCode, rm.RmGroupCode,
					rm.RmType,
					rm.RouteRmName, rm.RouteRmItemCode, rm.RouteRmShadeCode, rm.RouteRmShadeName,
					rm.RouteRmRatio, rm.UomID, rm.SubType, rm.Notes,
					actor,
				); err != nil {
					return nil, fmt.Errorf("update rm %d: %w", rm.RmID, err)
				}
			}
		}
	}

	// 5. Touch head's updated_at/by.
	if _, err := tx.ExecContext(ctx, `UPDATE cost_route_head SET crh_updated_at=now(), crh_updated_by=$2 WHERE crh_head_id=$1`, headID, actor); err != nil {
		return nil, fmt.Errorf("touch head: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit save graph tx: %w", err)
	}
	committed = true

	return r.GetGraph(ctx, headID)
}

// SaveHead persists status transitions and lock tracking on the head.
func (r *CostRouteRepository) SaveHead(ctx context.Context, head *costroute.Head, actor string) error {
	var lockedAt, unlockedAt sql.NullTime
	if !head.LockedAt.IsZero() {
		lockedAt = sql.NullTime{Time: head.LockedAt, Valid: true}
	}
	if !head.UnlockedAt.IsZero() {
		unlockedAt = sql.NullTime{Time: head.UnlockedAt, Valid: true}
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_route_head SET
			crh_routing_status=$2,
			crh_notes=NULLIF($3,''),
			crh_locked_by=NULLIF($5,''), crh_locked_at=$6,
			crh_unlocked_by=NULLIF($7,''), crh_unlocked_at=$8,
			crh_updated_at=now(), crh_updated_by=$4
		WHERE crh_head_id=$1 AND crh_deleted_at IS NULL`,
		head.HeadID, head.RoutingStatus, head.Notes, actor,
		head.LockedBy, lockedAt,
		head.UnlockedBy, unlockedAt,
	)
	if err != nil {
		return fmt.Errorf("save route head: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("save route head rows: %w", err)
	}
	if n == 0 {
		return costroute.ErrNotFound
	}
	return nil
}

// DeleteHead soft-deletes the head (cascade rules on FK remain).
func (r *CostRouteRepository) DeleteHead(ctx context.Context, headID int64, actor string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_route_head SET
			crh_deleted_at=now(), crh_deleted_by=$2
		WHERE crh_head_id=$1 AND crh_deleted_at IS NULL`,
		headID, actor,
	)
	if err != nil {
		return fmt.Errorf("soft-delete route head: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete head rows: %w", err)
	}
	if n == 0 {
		return costroute.ErrNotFound
	}
	return nil
}

// ListHeads applies a paginated filter.
func (r *CostRouteRepository) ListHeads(ctx context.Context, f costroute.Filter) ([]*costroute.Head, int64, error) { //nolint:gocognit,gocyclo // filter + pagination builder
	where := []string{"h.crh_deleted_at IS NULL"}
	args := []any{}
	idx := 1
	if f.Search != "" {
		where = append(where, fmt.Sprintf(`(LOWER(p.cpm_product_code) LIKE LOWER($%d) OR LOWER(p.cpm_product_name) LIKE LOWER($%d))`, idx, idx))
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.Status != "" {
		where = append(where, fmt.Sprintf(`h.crh_routing_status = $%d`, idx))
		args = append(args, f.Status)
		idx++
	}
	whereSQL := ""
	for i, w := range where {
		if i == 0 {
			whereSQL = " WHERE " + w
		} else {
			whereSQL += " AND " + w
		}
	}
	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	orderBy := "h.crh_created_at DESC"
	switch f.SortBy {
	case "product_code":
		orderBy = "p.cpm_product_code"
	case "status":
		orderBy = "h.crh_routing_status"
	case sortKeyCreatedAt, "":
		orderBy = "h.crh_created_at"
	}
	if f.SortOrder == "desc" || (f.SortOrder == "" && f.SortBy == "") {
		orderBy += " DESC"
	} else if f.SortOrder == "asc" {
		orderBy += " ASC"
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM cost_route_head h LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.crh_product_sys_id`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count routes: %w", err)
	}
	listSQL := `
		SELECT h.crh_head_id, h.crh_product_sys_id,
		       COALESCE(p.cpm_product_code, ''), COALESCE(p.cpm_product_name, ''),
		       h.crh_routing_status, h.crh_version,
		       COALESCE(h.crh_promoted_from_draft_id, 0), COALESCE(h.crh_cyl_type_id, 0),
		       COALESCE(h.crh_notes, ''),
		       h.crh_created_at, h.crh_created_by, h.crh_updated_at, COALESCE(h.crh_updated_by, ''),
		       COALESCE(h.crh_locked_by, ''), h.crh_locked_at,
		       COALESCE(h.crh_unlocked_by, ''), h.crh_unlocked_at
		FROM cost_route_head h
		LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.crh_product_sys_id` + whereSQL + ` ORDER BY ` + orderBy + fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costroute.Head{}
	for rows.Next() {
		h := &costroute.Head{}
		var lockedAt, unlockedAt sql.NullTime
		if err := rows.Scan(&h.HeadID, &h.ProductSysID, &h.ProductCode, &h.ProductName,
			&h.RoutingStatus, &h.Version,
			&h.PromotedFromDraftID, &h.CylTypeID, &h.Notes,
			&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
			&h.LockedBy, &lockedAt,
			&h.UnlockedBy, &unlockedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route row: %w", err)
		}
		if lockedAt.Valid {
			h.LockedAt = lockedAt.Time
		}
		if unlockedAt.Valid {
			h.UnlockedAt = unlockedAt.Time
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate route rows: %w", err)
	}
	return out, total, nil
}

// =============================================================================
// DuplicateRoute / ListLinkedRequests
// =============================================================================

// DuplicateRoute performs a transactional deep-copy per DuplicateInput's toggles.
func (r *CostRouteRepository) DuplicateRoute(ctx context.Context, in costroute.DuplicateInput) (costroute.DuplicateOutput, error) { //nolint:gocognit,gocyclo // cohesive transactional deep-copy
	if !in.IncludeApplicability && in.IncludeValues {
		return costroute.DuplicateOutput{}, fmt.Errorf("invalid input: values requested without applicability")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return costroute.DuplicateOutput{}, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	// 1. Load source head.
	var sourceHead struct {
		productSysID int64
		productCode  string
		productName  string
		cylTypeID    int32
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT h.crh_product_sys_id, COALESCE(p.cpm_product_code,''), COALESCE(p.cpm_product_name,''), COALESCE(h.crh_cyl_type_id, 0)
		FROM cost_route_head h
		LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.crh_product_sys_id
		WHERE h.crh_head_id = $1 AND h.crh_deleted_at IS NULL`, in.SourceHeadID,
	).Scan(&sourceHead.productSysID, &sourceHead.productCode, &sourceHead.productName, &sourceHead.cylTypeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return costroute.DuplicateOutput{}, costroute.ErrNotFound
		}
		return costroute.DuplicateOutput{}, fmt.Errorf("load source head: %w", err)
	}

	// 2. Generate FG fork code.
	newFGCode, err := r.generateForkedCode(ctx, tx, in.NewCodePrefix, sourceHead.productCode)
	if err != nil {
		return costroute.DuplicateOutput{}, err
	}

	// 3. BFS upstream products only if both flags on.
	upstreamProductIDs := []int64{}
	if in.IncludeUpstream && in.IncludeRouting { //nolint:nestif // BFS upstream product walk
		seen := map[int64]bool{sourceHead.productSysID: true}
		queue := []int64{sourceHead.productSysID}
		for len(queue) > 0 {
			pid := queue[0]
			queue = queue[1:]
			rows, qErr := tx.QueryContext(ctx, `
				SELECT DISTINCT rm.crm_rm_product_sys_id
				FROM cost_route_seq s
				JOIN cost_route_rm rm ON rm.crm_seq_id = s.crs_seq_id
				WHERE s.crs_head_id = $1
				  AND s.crs_product_sys_id = $2
				  AND rm.crm_rm_type = 'PRODUCT'
				  AND rm.crm_rm_product_sys_id IS NOT NULL`,
				in.SourceHeadID, pid)
			if qErr != nil {
				return costroute.DuplicateOutput{}, fmt.Errorf("walk upstream of %d: %w", pid, qErr)
			}
			for rows.Next() {
				var upstream int64
				if sErr := rows.Scan(&upstream); sErr != nil {
					if cErr := rows.Close(); cErr != nil {
						_ = cErr
					}
					return costroute.DuplicateOutput{}, fmt.Errorf("scan upstream: %w", sErr)
				}
				if !seen[upstream] {
					seen[upstream] = true
					upstreamProductIDs = append(upstreamProductIDs, upstream)
					queue = append(queue, upstream)
				}
			}
			if cErr := rows.Close(); cErr != nil {
				_ = cErr
			}
		}
	}

	// 4. Duplicate FG product master.
	productMap := map[int64]int64{}
	newFGSysID, err := r.duplicateProductTx(ctx, tx, sourceHead.productSysID, newFGCode, in.IncludeApplicability, in.IncludeValues, in.ActorUserID)
	if err != nil {
		return costroute.DuplicateOutput{}, err
	}
	productMap[sourceHead.productSysID] = newFGSysID

	// 5. Duplicate upstream products.
	for _, upstream := range upstreamProductIDs {
		newCode, gErr := r.generateForkedCode(ctx, tx, in.NewCodePrefix, "")
		if gErr != nil {
			return costroute.DuplicateOutput{}, gErr
		}
		newID, dErr := r.duplicateProductTx(ctx, tx, upstream, newCode, in.IncludeApplicability, in.IncludeValues, in.ActorUserID)
		if dErr != nil {
			return costroute.DuplicateOutput{}, dErr
		}
		productMap[upstream] = newID
	}

	// 6. Create new route head.
	var newHeadID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_version,
			crh_cyl_type_id, crh_forked_from_head_id,
			crh_created_by, crh_updated_by
		) VALUES ($1, 'DRAFT', 1, NULLIF($2,0)::integer, $3, $4, $4)
		RETURNING crh_head_id`,
		newFGSysID, sourceHead.cylTypeID, in.SourceHeadID, in.ActorUserID,
	).Scan(&newHeadID); err != nil {
		if isRouteUniqueViolation(err) {
			return costroute.DuplicateOutput{}, costroute.ErrAlreadyExists
		}
		return costroute.DuplicateOutput{}, fmt.Errorf("insert forked head: %w", err)
	}

	// 7. Optionally copy seqs + rms (remapping product references).
	if in.IncludeRouting {
		if err := r.duplicateGraphTx(ctx, tx, in.SourceHeadID, newHeadID, productMap, in.ActorUserID); err != nil {
			return costroute.DuplicateOutput{}, err
		}
	}

	// 8. Optionally update linked request atomically.
	if in.LinkedRequestID > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE cost_product_request
			SET cpr_linked_route_head_id = $2,
			    cpr_existing_product_sys_id = NULL,
			    cpr_updated_at = now()
			WHERE cpr_request_id = $1`,
			in.LinkedRequestID, newHeadID); err != nil {
			return costroute.DuplicateOutput{}, fmt.Errorf("update linked request: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return costroute.DuplicateOutput{}, fmt.Errorf("commit duplicate tx: %w", err)
	}
	committed = true

	return costroute.DuplicateOutput{
		NewHeadID:       newHeadID,
		NewProductSysID: newFGSysID,
		NewProductCode:  newFGCode,
	}, nil
}

const maxProductCodeLen = 20

// generateForkedCode finds the next unique product code derived from base.
// Each candidate is at most 20 chars: the numeric suffix is allocated first
// so a long base never produces duplicate truncated candidates.
func (r *CostRouteRepository) generateForkedCode(ctx context.Context, tx *sql.Tx, prefix, sourceCode string) (string, error) {
	base := prefix
	if base == "" {
		if sourceCode != "" {
			base = sourceCode + "_F"
		} else {
			base = "FORK"
		}
	}
	// Truncate base to maxProductCodeLen so the suffix can always be appended.
	if len(base) >= maxProductCodeLen {
		base = base[:maxProductCodeLen-1]
	}
	for n := 1; n <= 9999; n++ {
		suffix := fmt.Sprintf("%d", n)
		// Ensure base + suffix fits within limit.
		b := base
		if len(b)+len(suffix) > maxProductCodeLen {
			b = base[:maxProductCodeLen-len(suffix)]
		}
		candidate := b + suffix
		var taken bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM cost_product_master WHERE cpm_product_code=$1)`,
			candidate,
		).Scan(&taken); err != nil {
			return "", fmt.Errorf("check candidate code: %w", err)
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not generate unique forked code from base %q", base)
}

// duplicateProductTx copies a product master + optional applicability + values.
func (r *CostRouteRepository) duplicateProductTx(
	ctx context.Context, tx *sql.Tx,
	sourceProductSysID int64, newCode string,
	includeApplicability, includeValues bool,
	actor string,
) (int64, error) {
	var newSysID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO cost_product_master (
			cpm_product_code, cpm_product_type_id, cpm_product_name,
			cpm_shade_code, cpm_grade_code, cpm_description,
			cpm_is_active, cpm_created_by, cpm_updated_by
		)
		SELECT $1, cpm_product_type_id, cpm_product_name,
		       cpm_shade_code, cpm_grade_code, cpm_description,
		       TRUE, $2, $2
		FROM cost_product_master
		WHERE cpm_product_sys_id = $3
		RETURNING cpm_product_sys_id`,
		newCode, actor, sourceProductSysID,
	).Scan(&newSysID); err != nil {
		return 0, fmt.Errorf("duplicate product master %d: %w", sourceProductSysID, err)
	}
	if includeApplicability {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cost_product_applicable_param (
				capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
			)
			SELECT $1, capp_param_id, capp_is_required, capp_display_order, $2
			FROM cost_product_applicable_param
			WHERE capp_product_sys_id = $3`,
			newSysID, actor, sourceProductSysID); err != nil {
			return 0, fmt.Errorf("copy capp applicability: %w", err)
		}
	}
	if includeValues {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cost_product_parameter (
				cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_value_text, cpp_value_flag,
				cpp_filled_by, cpp_created_by
			)
			SELECT $1, cpp_param_id, cpp_value_numeric, cpp_value_text, cpp_value_flag, $2, $2
			FROM cost_product_parameter
			WHERE cpp_product_sys_id = $3`,
			newSysID, actor, sourceProductSysID); err != nil {
			return 0, fmt.Errorf("copy capp values: %w", err)
		}
	}
	return newSysID, nil
}

// duplicateGraphTx copies seqs + rms, remapping product references via productMap.
func (r *CostRouteRepository) duplicateGraphTx( //nolint:gocognit,gocyclo // cohesive transactional graph copy
	ctx context.Context, tx *sql.Tx,
	sourceHeadID, newHeadID int64,
	productMap map[int64]int64,
	actor string,
) error {
	type srcSeq struct {
		id             int64
		productID      int64
		level, seq     int32
		name, itemC    string
		shadeC, shadeN string
		x, y           float64
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT crs_seq_id, crs_product_sys_id, crs_route_level, crs_route_seq,
		       COALESCE(crs_route_name,''), COALESCE(crs_route_item_code,''),
		       COALESCE(crs_route_shade_code,''), COALESCE(crs_route_shade_name,''),
		       crs_position_x, crs_position_y
		FROM cost_route_seq
		WHERE crs_head_id = $1 AND crs_deleted_at IS NULL
		ORDER BY crs_route_level, crs_route_seq`, sourceHeadID)
	if err != nil {
		return fmt.Errorf("load source seqs: %w", err)
	}
	seqs := []srcSeq{}
	for rows.Next() {
		var s srcSeq
		if scanErr := rows.Scan(&s.id, &s.productID, &s.level, &s.seq, &s.name, &s.itemC, &s.shadeC, &s.shadeN, &s.x, &s.y); scanErr != nil {
			if cErr := rows.Close(); cErr != nil {
				_ = cErr
			}
			return fmt.Errorf("scan seq: %w", scanErr)
		}
		seqs = append(seqs, s)
	}
	if cErr := rows.Close(); cErr != nil {
		_ = cErr
	}

	seqMap := map[int64]int64{}
	for _, s := range seqs {
		newProductID := s.productID
		if mapped, ok := productMap[s.productID]; ok {
			newProductID = mapped
		}
		var newSeqID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO cost_route_seq (
				crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
				crs_route_name, crs_route_item_code, crs_route_shade_code, crs_route_shade_name,
				crs_position_x, crs_position_y,
				crs_created_by, crs_updated_by
			) VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10,$11,$11)
			RETURNING crs_seq_id`,
			newHeadID, newProductID, s.level, s.seq, s.name, s.itemC, s.shadeC, s.shadeN, s.x, s.y, actor,
		).Scan(&newSeqID); err != nil {
			return fmt.Errorf("insert duplicated seq L%d/%d: %w", s.level, s.seq, err)
		}
		seqMap[s.id] = newSeqID
	}

	// Copy RMs, remapping product references.
	// First load ALL source rows into memory, then INSERT — same database connection
	// cannot interleave a cursor read with parameterized INSERT (database/sql
	// returns "bad connection" when the row cursor is still open).
	type srcRm struct {
		oldSeqID, parentProd                                int64
		rmProd                                              sql.NullInt64
		itemC, groupC                                       string
		rmType, name, itemCode, shadeC, shadeN, subT, notes string
		ratio                                               float64
		uomID                                               int32
	}
	rmRows, err := tx.QueryContext(ctx, `
		SELECT rm.crm_seq_id, rm.crm_parent_product_sys_id,
		       rm.crm_rm_product_sys_id, COALESCE(rm.crm_rm_item_code,''), COALESCE(rm.crm_rm_group_code,''),
		       rm.crm_rm_type,
		       COALESCE(rm.crm_route_rm_name,''), COALESCE(rm.crm_route_rm_item_code,''),
		       COALESCE(rm.crm_route_rm_shade_code,''), COALESCE(rm.crm_route_rm_shade_name,''),
		       rm.crm_route_rm_ratio, COALESCE(rm.crm_uom_id, 0), COALESCE(rm.crm_sub_type,''), COALESCE(rm.crm_notes,'')
		FROM cost_route_rm rm
		JOIN cost_route_seq s ON s.crs_seq_id = rm.crm_seq_id
		WHERE s.crs_head_id = $1`, sourceHeadID)
	if err != nil {
		return fmt.Errorf("load source rms: %w", err)
	}
	srcRms := []srcRm{}
	for rmRows.Next() {
		var rm srcRm
		if err := rmRows.Scan(&rm.oldSeqID, &rm.parentProd, &rm.rmProd, &rm.itemC, &rm.groupC, &rm.rmType,
			&rm.name, &rm.itemCode, &rm.shadeC, &rm.shadeN, &rm.ratio, &rm.uomID, &rm.subT, &rm.notes); err != nil {
			if cerr := rmRows.Close(); cerr != nil {
				_ = cerr
			}
			return fmt.Errorf("scan rm: %w", err)
		}
		srcRms = append(srcRms, rm)
	}
	if err := rmRows.Err(); err != nil {
		if cerr := rmRows.Close(); cerr != nil {
			_ = cerr
		}
		return fmt.Errorf("iterate source rms: %w", err)
	}
	if cErr := rmRows.Close(); cErr != nil {
		return fmt.Errorf("close source rms cursor: %w", cErr)
	}

	for _, rm := range srcRms {
		newSeqID := seqMap[rm.oldSeqID]
		newParent := rm.parentProd
		if mapped, ok := productMap[rm.parentProd]; ok {
			newParent = mapped
		}
		var newRmProdSysID sql.NullInt64
		if rm.rmProd.Valid {
			if mapped, ok := productMap[rm.rmProd.Int64]; ok {
				newRmProdSysID = sql.NullInt64{Int64: mapped, Valid: true}
			} else {
				newRmProdSysID = rm.rmProd
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO cost_route_rm (
				crm_seq_id, crm_parent_product_sys_id,
				crm_rm_product_sys_id, crm_rm_item_code, crm_rm_group_code,
				crm_rm_type,
				crm_route_rm_name, crm_route_rm_item_code, crm_route_rm_shade_code, crm_route_rm_shade_name,
				crm_route_rm_ratio, crm_uom_id, crm_sub_type, crm_notes,
				crm_created_by, crm_updated_by
			) VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,
			          NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),
			          $11,NULLIF($12,0)::integer,NULLIF($13,''),NULLIF($14,''),$15,$15)`,
			newSeqID, newParent, newRmProdSysID, rm.itemC, rm.groupC, rm.rmType,
			rm.name, rm.itemCode, rm.shadeC, rm.shadeN, rm.ratio, rm.uomID, rm.subT, rm.notes, actor); err != nil {
			return fmt.Errorf("insert duplicated rm: %w", err)
		}
	}
	return nil
}

// ListLinkedRequests returns all requests linking to this route head.
// NOTE: cost_product_request has no cpr_product_top_2 column in this schema;
// the LinkedRequest.ProductTop2 field is left empty.
func (r *CostRouteRepository) ListLinkedRequests(ctx context.Context, headID int64) ([]costroute.LinkedRequest, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cpr_request_id, cpr_request_no, cpr_status,
		       cpr_requester_user_id, cpr_created_at
		FROM cost_product_request
		WHERE cpr_linked_route_head_id = $1
		ORDER BY cpr_created_at DESC
		LIMIT 200`, headID)
	if err != nil {
		return nil, fmt.Errorf("list linked requests: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			_ = cErr
		}
	}()
	out := []costroute.LinkedRequest{}
	for rows.Next() {
		var lr costroute.LinkedRequest
		if err := rows.Scan(&lr.RequestID, &lr.RequestNo, &lr.Status, &lr.CreatedBy, &lr.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan linked request: %w", err)
		}
		out = append(out, lr)
	}
	return out, rows.Err()
}

// =============================================================================
// Bulk import helpers (BulkUpsertHeads, BulkUpsertSeqs, BulkReplaceRMs)
// =============================================================================

// BulkUpsertHeads upserts route head rows by (crh_product_sys_id).
// Rows whose existing crh_routing_status is 'LOCKED' are returned with Skipped=true.
func (r *CostRouteRepository) BulkUpsertHeads(ctx context.Context, items []costroute.HeadUpsertInput, actor string) ([]costroute.HeadUpsertResult, error) {
	if len(items) == 0 {
		return []costroute.HeadUpsertResult{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin BulkUpsertHeads tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	const q = `
		INSERT INTO cost_route_head (
			crh_product_sys_id, crh_routing_status, crh_notes,
			crh_created_at, crh_created_by, crh_updated_at, crh_updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $4, $5)
		ON CONFLICT (crh_product_sys_id) WHERE crh_deleted_at IS NULL AND crh_routing_status <> 'LOCKED'
		DO UPDATE SET
			crh_notes      = CASE WHEN cost_route_head.crh_routing_status = 'LOCKED' THEN cost_route_head.crh_notes ELSE EXCLUDED.crh_notes END,
			crh_updated_at = CASE WHEN cost_route_head.crh_routing_status = 'LOCKED' THEN cost_route_head.crh_updated_at ELSE EXCLUDED.crh_updated_at END,
			crh_updated_by = CASE WHEN cost_route_head.crh_routing_status = 'LOCKED' THEN cost_route_head.crh_updated_by ELSE EXCLUDED.crh_updated_by END
		RETURNING crh_head_id, xmax::text, crh_routing_status`

	results := make([]costroute.HeadUpsertResult, 0, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		status := item.RoutingStatus
		if status == "" {
			status = costroute.StatusDraft
		}
		var headID int64
		var xmax, routingStatus string
		if err := tx.QueryRowContext(ctx, q,
			item.ProductSysID, status, item.Notes, now, actor,
		).Scan(&headID, &xmax, &routingStatus); err != nil {
			return nil, fmt.Errorf("BulkUpsertHeads upsert row: %w", err)
		}
		results = append(results, costroute.HeadUpsertResult{
			LegacySysID: item.LegacySysID,
			HeadID:      headID,
			WasInserted: xmax == "0",
			Skipped:     routingStatus == costroute.StatusLocked && xmax != "0",
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit BulkUpsertHeads: %w", err)
	}
	return results, nil
}

// BulkUpsertSeqs upserts route sequence rows by (crs_head_id, crs_route_level, crs_route_seq).
func (r *CostRouteRepository) BulkUpsertSeqs(ctx context.Context, items []costroute.SeqUpsertInput, actor string) ([]costroute.SeqUpsertResult, error) {
	if len(items) == 0 {
		return []costroute.SeqUpsertResult{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin BulkUpsertSeqs tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	const q = `
		INSERT INTO cost_route_seq (
			crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
			crs_route_name, crs_route_item_code, crs_route_shade_code, crs_route_shade_name,
			crs_created_at, crs_created_by, crs_updated_at, crs_updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $9, $10)
		ON CONFLICT (crs_head_id, crs_route_level, crs_route_seq)
		DO UPDATE SET
			crs_product_sys_id   = EXCLUDED.crs_product_sys_id,
			crs_route_name       = EXCLUDED.crs_route_name,
			crs_route_item_code  = EXCLUDED.crs_route_item_code,
			crs_route_shade_code = EXCLUDED.crs_route_shade_code,
			crs_route_shade_name = EXCLUDED.crs_route_shade_name,
			crs_updated_at       = EXCLUDED.crs_updated_at,
			crs_updated_by       = EXCLUDED.crs_updated_by
		RETURNING crs_seq_id, xmax::text`

	results := make([]costroute.SeqUpsertResult, 0, len(items))
	now := time.Now().UTC()
	for _, item := range items {
		var seqID int64
		var xmax string
		if err := tx.QueryRowContext(ctx, q,
			item.HeadID, item.NodeProductSysID, item.RouteLevel, item.RouteSeq,
			item.RouteName, item.RouteItemCode, item.RouteShadeCode, item.RouteShadeName,
			now, actor,
		).Scan(&seqID, &xmax); err != nil {
			return nil, fmt.Errorf("BulkUpsertSeqs upsert row: %w", err)
		}
		results = append(results, costroute.SeqUpsertResult{
			LegacySysID: item.HeadLegacySysID,
			SeqID:       seqID,
			HeadID:      item.HeadID,
			RouteLevel:  item.RouteLevel,
			RouteSeq:    item.RouteSeq,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit BulkUpsertSeqs: %w", err)
	}
	return results, nil
}

// BulkReplaceRMs deletes all existing RMs for seqID and re-inserts the given rms in a single transaction.
func (r *CostRouteRepository) BulkReplaceRMs(ctx context.Context, seqID int64, rms []costroute.RMInput, actor string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin BulkReplaceRMs tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM cost_route_rm WHERE crm_seq_id = $1`, seqID); err != nil {
		return fmt.Errorf("BulkReplaceRMs delete: %w", err)
	}

	const insertRM = `
		INSERT INTO cost_route_rm (
			crm_seq_id, crm_rm_type,
			crm_rm_product_sys_id, crm_rm_item_code, crm_rm_group_code,
			crm_route_rm_ratio, crm_route_rm_name,
			crm_route_rm_shade_code, crm_route_rm_shade_name,
			crm_sub_type, crm_notes,
			crm_created_at, crm_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	now := time.Now().UTC()
	for _, rm := range rms {
		var rmProductSysID sql.NullInt64
		if rm.RmType == costroute.RmTypeProduct && rm.RmProductSysID != 0 {
			rmProductSysID = sql.NullInt64{Int64: rm.RmProductSysID, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, insertRM,
			seqID, rm.RmType,
			rmProductSysID, nullableString(rm.RmItemCode), nullableString(rm.RmGroupCode),
			rm.Ratio, nullableString(rm.RmName),
			nullableString(rm.RmShadeCode), nullableString(rm.RmShadeName),
			nullableString(rm.SubType), nullableString(rm.Notes),
			now, actor,
		); err != nil {
			return fmt.Errorf("BulkReplaceRMs insert rm: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit BulkReplaceRMs: %w", err)
	}
	return nil
}

// ListAllHeadsForExport returns all non-deleted route heads for export, optionally
// filtered to the given product sys IDs. An empty productSysIDs slice returns all heads.
func (r *CostRouteRepository) ListAllHeadsForExport(ctx context.Context, productSysIDs []int64) ([]costroute.ExportRouteHead, error) {
	q := `SELECT crh_head_id, crh_product_sys_id, crh_routing_status, COALESCE(crh_notes,'')
          FROM cost_route_head
          WHERE crh_deleted_at IS NULL`
	var args []any
	if len(productSysIDs) > 0 {
		q += ` AND crh_product_sys_id = ANY($1)`
		args = append(args, pq.Array(productSysIDs))
	}
	q += ` ORDER BY crh_head_id`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list all route heads for export: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	var out []costroute.ExportRouteHead
	for rows.Next() {
		var h costroute.ExportRouteHead
		if scanErr := rows.Scan(&h.HeadID, &h.ProductSysID, &h.RoutingStatus, &h.Notes); scanErr != nil {
			return nil, fmt.Errorf("scan export route head: %w", scanErr)
		}
		out = append(out, h)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate export route heads: %w", rowsErr)
	}
	return out, nil
}

// ListAllSeqsForExport returns all non-deleted route seqs for the given head IDs.
func (r *CostRouteRepository) ListAllSeqsForExport(ctx context.Context, headIDs []int64) ([]costroute.ExportRouteSeq, error) {
	if len(headIDs) == 0 {
		return nil, nil
	}
	const q = `
SELECT crs_seq_id, crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
       COALESCE(crs_route_name,''), COALESCE(crs_route_item_code,''),
       COALESCE(crs_route_shade_code,''), COALESCE(crs_route_shade_name,'')
FROM cost_route_seq
WHERE crs_head_id = ANY($1) AND crs_deleted_at IS NULL
ORDER BY crs_head_id, crs_route_level, crs_route_seq`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(headIDs))
	if err != nil {
		return nil, fmt.Errorf("list all route seqs for export: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	var out []costroute.ExportRouteSeq
	for rows.Next() {
		var s costroute.ExportRouteSeq
		if scanErr := rows.Scan(
			&s.SeqID, &s.HeadID, &s.ProductSysID, &s.RouteLevel, &s.RouteSeq,
			&s.RouteName, &s.RouteItemCode, &s.RouteShadeCode, &s.RouteShadeName,
		); scanErr != nil {
			return nil, fmt.Errorf("scan export route seq: %w", scanErr)
		}
		out = append(out, s)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate export route seqs: %w", rowsErr)
	}
	return out, nil
}

// ListAllRMsForExport returns all route RMs for the given seq IDs.
// The HeadID field is populated from the caller-supplied seq→head mapping.
func (r *CostRouteRepository) ListAllRMsForExport(ctx context.Context, seqIDs []int64) ([]costroute.ExportRouteRM, error) {
	if len(seqIDs) == 0 {
		return nil, nil
	}
	const q = `
SELECT rm.crm_seq_id,
       COALESCE(s.crs_route_level, 0), COALESCE(s.crs_route_seq, 0),
       rm.crm_rm_type, COALESCE(rm.crm_rm_product_sys_id, 0),
       COALESCE(rm.crm_rm_item_code,''), COALESCE(rm.crm_rm_group_code,''),
       rm.crm_route_rm_ratio, COALESCE(rm.crm_route_rm_name,''),
       COALESCE(rm.crm_route_rm_shade_code,''), COALESCE(rm.crm_route_rm_shade_name,''),
       COALESCE(rm.crm_sub_type,''), COALESCE(rm.crm_notes,'')
FROM cost_route_rm rm
LEFT JOIN cost_route_seq s ON s.crs_seq_id = rm.crm_seq_id
WHERE rm.crm_seq_id = ANY($1)
ORDER BY rm.crm_seq_id`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(seqIDs))
	if err != nil {
		return nil, fmt.Errorf("list all route rms for export: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	var out []costroute.ExportRouteRM
	for rows.Next() {
		var rm costroute.ExportRouteRM
		if scanErr := rows.Scan(
			&rm.SeqID,
			&rm.RouteLevel, &rm.RouteSeq,
			&rm.RmType, &rm.RmProductSysID,
			&rm.RmItemCode, &rm.RmGroupCode,
			&rm.Ratio, &rm.RmName,
			&rm.RmShadeCode, &rm.RmShadeName,
			&rm.SubType, &rm.Notes,
		); scanErr != nil {
			return nil, fmt.Errorf("scan export route rm: %w", scanErr)
		}
		out = append(out, rm)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate export route rms: %w", rowsErr)
	}
	return out, nil
}
