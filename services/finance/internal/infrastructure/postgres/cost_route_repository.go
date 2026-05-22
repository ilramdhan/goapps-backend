package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
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
		       crh_created_at, crh_created_by, crh_updated_at, COALESCE(crh_updated_by, '')
		FROM cost_route_head
		WHERE crh_product_sys_id = $1
		  AND crh_deleted_at IS NULL
		  AND crh_routing_status <> 'LOCKED'
		LIMIT 1`
	h := &costroute.Head{}
	err := r.db.QueryRowContext(ctx, q, productSysID).Scan(
		&h.HeadID, &h.ProductSysID, &h.RoutingStatus, &h.Version,
		&h.PromotedFromDraftID, &h.CylTypeID,
		&h.Notes,
		&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costroute.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active route by product: %w", err)
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
		       h.crh_created_at, h.crh_created_by, h.crh_updated_at, COALESCE(h.crh_updated_by, '')
		FROM cost_route_head h
		LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.crh_product_sys_id
		WHERE h.crh_head_id = $1 AND h.crh_deleted_at IS NULL`
	h := &costroute.Head{}
	err := r.db.QueryRowContext(ctx, q, headID).Scan(
		&h.HeadID, &h.ProductSysID, &h.ProductCode, &h.ProductName,
		&h.RoutingStatus, &h.Version,
		&h.PromotedFromDraftID, &h.CylTypeID, &h.Notes,
		&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costroute.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get route head: %w", err)
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
	defer rows.Close()
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
	defer rows.Close()
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
func (r *CostRouteRepository) SaveGraph(ctx context.Context, headID int64, in *costroute.Graph, actor string) (*costroute.Graph, error) {
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
			rowsToDelete.Close()
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
				rowsR.Close()
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

// SaveHead persists status transitions on the head.
func (r *CostRouteRepository) SaveHead(ctx context.Context, head *costroute.Head, actor string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_route_head SET
			crh_routing_status=$2,
			crh_notes=NULLIF($3,''),
			crh_updated_at=now(), crh_updated_by=$4
		WHERE crh_head_id=$1 AND crh_deleted_at IS NULL`,
		head.HeadID, head.RoutingStatus, head.Notes, actor,
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
func (r *CostRouteRepository) ListHeads(ctx context.Context, f costroute.Filter) ([]*costroute.Head, int64, error) {
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
	page := f.Page
	if page < 1 {
		page = 1
	}
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
	case "created_at", "":
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
		       h.crh_created_at, h.crh_created_by, h.crh_updated_at, COALESCE(h.crh_updated_by, '')
		FROM cost_route_head h
		LEFT JOIN cost_product_master p ON p.cpm_product_sys_id = h.crh_product_sys_id` + whereSQL + ` ORDER BY ` + orderBy + fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()
	out := []*costroute.Head{}
	for rows.Next() {
		h := &costroute.Head{}
		if err := rows.Scan(&h.HeadID, &h.ProductSysID, &h.ProductCode, &h.ProductName,
			&h.RoutingStatus, &h.Version,
			&h.PromotedFromDraftID, &h.CylTypeID, &h.Notes,
			&h.CreatedAt, &h.CreatedBy, &h.UpdatedAt, &h.UpdatedBy,
		); err != nil {
			return nil, 0, fmt.Errorf("scan route row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate route rows: %w", err)
	}
	return out, total, nil
}
