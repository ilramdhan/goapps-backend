package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// ERP link persistence (ERP cost integration P3-T5, D-LINK). The link key is
// (cpm_erp_item_code, cpm_shade_code), compared trimmed and upper-cased; a
// NULL shade equals ''. "AX" means COALESCE(NULLIF(cpm_grade_code,''),'AX')
// = 'AX', the same predicate as the gated unique index 000556 (design §4.9).
// cpm_erp_grade_code_1/2 are never written here.

var _ costproductmaster.ErpLinkRepository = (*CostProductMasterRepository)(nil)

// cpmErpKeyPredicate matches active AX products holding the key ($1 item, $2
// shade key) other than $3.
const cpmErpKeyPredicate = `
	cpm_is_active
	AND cpm_erp_item_code IS NOT NULL
	AND UPPER(TRIM(cpm_erp_item_code)) = UPPER($1)
	AND UPPER(TRIM(COALESCE(cpm_shade_code,''))) = $2
	AND UPPER(COALESCE(NULLIF(TRIM(cpm_grade_code),''),'AX')) = 'AX'
	AND cpm_product_sys_id <> $3`

// cpmErpLinkLockNS namespaces the per-key advisory lock of SaveErpLink.
const cpmErpLinkLockNS = "cpm_erp_link:"

type cpmErpKeyQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ListActiveAxByErpKey returns the other active AX products holding the key.
func (r *CostProductMasterRepository) ListActiveAxByErpKey(ctx context.Context, itemCode, shadeKey string, excludeSysID int64) ([]int64, error) {
	return listErpKeyHolders(ctx, r.db, itemCode, shadeKey, excludeSysID)
}

func listErpKeyHolders(ctx context.Context, q cpmErpKeyQuerier, itemCode, shadeKey string, excludeSysID int64) (ids []int64, err error) {
	rows, err := q.QueryContext(ctx,
		`SELECT cpm_product_sys_id FROM cost_product_master WHERE `+cpmErpKeyPredicate+` ORDER BY cpm_product_sys_id`,
		strings.TrimSpace(itemCode), costproductmaster.NormalizeShadeKey(shadeKey), excludeSysID)
	if err != nil {
		return nil, fmt.Errorf("list ERP key holders: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close ERP key holders: %w", cerr)
		}
	}()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan ERP key holder: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ERP key holders: %w", err)
	}
	return ids, nil
}

// SaveErpLink writes the link in one transaction: a per-key advisory lock
// serializes concurrent links of the same key, the row is re-read FOR UPDATE
// and must still carry PrevItemCode and ShadeKey (else ErrLinkStale), and a
// non-empty NewItemCode is re-checked for duplicates (ErrLinkDuplicate).
func (r *CostProductMasterRepository) SaveErpLink(ctx context.Context, w costproductmaster.ErpLinkWrite) (err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin ERP link tx: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Int64("product_sys_id", w.ProductSysID).Msg("rollback ERP link tx")
		}
	}()

	newItem := strings.TrimSpace(w.NewItemCode)
	shadeKey := costproductmaster.NormalizeShadeKey(w.ShadeKey)
	if newItem != "" {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			cpmErpLinkLockNS+strings.ToUpper(newItem)+"|"+shadeKey); err != nil {
			return fmt.Errorf("lock ERP key: %w", err)
		}
	}
	if err = checkErpLinkRow(ctx, tx, w.ProductSysID, w.PrevItemCode, shadeKey); err != nil {
		return err
	}
	if newItem != "" {
		dups, dErr := listErpKeyHolders(ctx, tx, newItem, shadeKey, w.ProductSysID)
		if dErr != nil {
			return dErr
		}
		if len(dups) > 0 {
			return fmt.Errorf("%w: product_sys_id %v", costproductmaster.ErrLinkDuplicate, dups)
		}
	}
	if err = execErpLinkUpdate(ctx, tx, w, newItem); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit ERP link tx: %w", err)
	}
	return nil
}

func checkErpLinkRow(ctx context.Context, tx *sql.Tx, sysID int64, prevItem, shadeKey string) error {
	var curItem, curShade string
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(TRIM(cpm_erp_item_code),''), UPPER(TRIM(COALESCE(cpm_shade_code,'')))
		FROM cost_product_master WHERE cpm_product_sys_id = $1 FOR UPDATE`, sysID).Scan(&curItem, &curShade)
	if errors.Is(err, sql.ErrNoRows) {
		return costproductmaster.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load product for ERP link: %w", err)
	}
	if curItem != strings.TrimSpace(prevItem) || curShade != shadeKey {
		return costproductmaster.ErrLinkStale
	}
	return nil
}

func execErpLinkUpdate(ctx context.Context, tx *sql.Tx, w costproductmaster.ErpLinkWrite, newItem string) error {
	var item, by sql.NullString
	var at sql.NullTime
	if newItem != "" {
		item = sql.NullString{String: newItem, Valid: true}
		by = sql.NullString{String: w.LinkedBy, Valid: w.LinkedBy != ""}
		if w.LinkedAt != nil {
			at = sql.NullTime{Time: *w.LinkedAt, Valid: true}
		}
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE cost_product_master SET
			cpm_erp_item_code = $2, cpm_erp_linked_at = $3, cpm_erp_linked_by = $4,
			cpm_updated_at = $5, cpm_updated_by = $6
		WHERE cpm_product_sys_id = $1`,
		w.ProductSysID, item, at, by, w.UpdatedAt, w.UpdatedBy)
	if err != nil {
		return fmt.Errorf("update ERP link: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ERP link rows affected: %w", err)
	}
	if n == 0 {
		return costproductmaster.ErrNotFound
	}
	return nil
}

// SaveErpAttributes writes the five 000551 columns ("" = NULL).
func (r *CostProductMasterRepository) SaveErpAttributes(ctx context.Context, w costproductmaster.ErpAttributesWrite) error {
	a := w.Attributes
	var prd any
	if a.PrdPerDay.Valid {
		prd = a.PrdPerDay.Decimal.String()
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_product_master SET
			cpm_erp_fg_type = NULLIF($2,''), cpm_erp_chp_item_code = NULLIF($3,''),
			cpm_erp_ms_batch_item = NULLIF($4,''), cpm_erp_item_type = NULLIF($5,''),
			cpm_erp_prd_per_day = $6::numeric,
			cpm_updated_at = $7, cpm_updated_by = $8
		WHERE cpm_product_sys_id = $1`,
		w.ProductSysID, a.FgType, a.ChpItemCode, a.MsBatchItem, a.ItemType, prd, w.UpdatedAt, w.UpdatedBy)
	if err != nil {
		return fmt.Errorf("update ERP attributes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ERP attributes rows affected: %w", err)
	}
	if n == 0 {
		return costproductmaster.ErrNotFound
	}
	return nil
}
