package postgres

// erp_attr_backfill_repository.go is the PostgreSQL side of the one-off ERP
// attribute backfill (plan P3-T6). It lists already-linked active AX products
// and fills NULL (or blank) 000551 attribute columns only. It never writes
// cpm_erp_item_code, cpm_erp_linked_*, or cpm_erp_grade_code_1/2, so no link
// is created, changed or removed, and a column holding a value is never
// overwritten (guarded in Go under the row lock and again in SQL).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// attrBackfillDefaultActor is written to cpm_updated_by when no actor is given.
const attrBackfillDefaultActor = "system"

// ErpAttrBackfillRepository implements erpintegration.AttrBackfillRepository.
type ErpAttrBackfillRepository struct {
	db  *DB
	now func() time.Time
}

var _ erpintegration.AttrBackfillRepository = (*ErpAttrBackfillRepository)(nil)

// NewErpAttrBackfillRepository builds the repository.
func NewErpAttrBackfillRepository(db *DB) *ErpAttrBackfillRepository {
	return &ErpAttrBackfillRepository{db: db, now: time.Now}
}

// cpmLinkedAxPredicate matches active AX products holding an ERP item code.
const cpmLinkedAxPredicate = `
	cpm_is_active
	AND NULLIF(TRIM(cpm_erp_item_code),'') IS NOT NULL
	AND UPPER(COALESCE(NULLIF(TRIM(cpm_grade_code),''),'AX')) = 'AX'`

// cpmAttrColumns reads the five attributes as text ("" = NULL).
const cpmAttrColumns = `
	COALESCE(TRIM(cpm_erp_fg_type),''), COALESCE(TRIM(cpm_erp_chp_item_code),''),
	COALESCE(TRIM(cpm_erp_ms_batch_item),''), COALESCE(TRIM(cpm_erp_item_type),''),
	COALESCE(cpm_erp_prd_per_day::text,'')`

// ListLinkedProducts implements erpintegration.AttrBackfillRepository.
func (r *ErpAttrBackfillRepository) ListLinkedProducts(ctx context.Context) (out []erpintegration.LinkedProductAttrs, err error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT cpm_product_sys_id, cpm_product_code, TRIM(cpm_erp_item_code), COALESCE(TRIM(cpm_shade_code),''),`+
		cpmAttrColumns+`
		FROM cost_product_master
		WHERE `+cpmLinkedAxPredicate+`
		ORDER BY cpm_product_sys_id`)
	if err != nil {
		return nil, fmt.Errorf("list linked products: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close linked products: %w", cerr)
		}
	}()
	for rows.Next() {
		var p erpintegration.LinkedProductAttrs
		a := &p.Attrs
		if err := rows.Scan(&p.ProductSysID, &p.ProductCode, &p.ErpItemCode, &p.ShadeCode,
			&a.FgType, &a.ChpItemCode, &a.MsBatchItem, &a.ItemType, &a.PrdPerDay); err != nil {
			return nil, fmt.Errorf("scan linked product: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked products: %w", err)
	}
	return out, nil
}

// FillNullAttributes implements erpintegration.AttrBackfillRepository.
func (r *ErpAttrBackfillRepository) FillNullAttributes(ctx context.Context, w erpintegration.AttrFillWrite) (res erpintegration.AttrFillResult, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return res, fmt.Errorf("begin attr backfill tx: %w", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			log.Warn().Err(rbErr).Int64("product_sys_id", w.ProductSysID).Msg("rollback attr backfill tx")
		}
	}()

	before, stale, err := lockAttrRow(ctx, tx, w)
	if err != nil {
		return res, err
	}
	res.Before, res.After = before, before
	if stale {
		res.Stale = true
		if err = tx.Commit(); err != nil {
			return res, fmt.Errorf("commit attr backfill tx: %w", err)
		}
		return res, nil
	}
	fill := erpintegration.AttrValues{}
	for _, f := range erpintegration.AllAttrFields() {
		v := strings.TrimSpace(w.Values.Get(f))
		if v != "" && before.Get(f) == "" {
			fill = fill.With(f, v)
			res.Filled = append(res.Filled, f)
		}
	}
	if len(res.Filled) > 0 {
		if res.After, err = execAttrFill(ctx, tx, w, fill, r.now()); err != nil {
			return res, err
		}
	}
	if err = tx.Commit(); err != nil {
		return res, fmt.Errorf("commit attr backfill tx: %w", err)
	}
	return res, nil
}

// lockAttrRow re-reads the product FOR UPDATE. It is stale when the product
// is gone, inactive, no longer AX, or no longer carries the planned link key.
func lockAttrRow(ctx context.Context, tx *sql.Tx, w erpintegration.AttrFillWrite) (erpintegration.AttrValues, bool, error) {
	var (
		v           erpintegration.AttrValues
		item, shade string
		active, ax  bool
	)
	err := tx.QueryRowContext(ctx, `
		SELECT cpm_is_active, UPPER(COALESCE(NULLIF(TRIM(cpm_grade_code),''),'AX')) = 'AX',
			COALESCE(TRIM(cpm_erp_item_code),''), UPPER(COALESCE(TRIM(cpm_shade_code),'')),`+
		cpmAttrColumns+`
		FROM cost_product_master WHERE cpm_product_sys_id = $1 FOR UPDATE`, w.ProductSysID).
		Scan(&active, &ax, &item, &shade, &v.FgType, &v.ChpItemCode, &v.MsBatchItem, &v.ItemType, &v.PrdPerDay)
	if errors.Is(err, sql.ErrNoRows) {
		return v, true, nil
	}
	if err != nil {
		return v, false, fmt.Errorf("load product for attr backfill: %w", err)
	}
	stale := !active || !ax || item == "" ||
		!strings.EqualFold(item, strings.TrimSpace(w.ErpItemCode)) ||
		shade != costproductmaster.NormalizeShadeKey(w.ShadeKey)
	return v, stale, nil
}

// execAttrFill writes the fill values. Each column is set only while it is
// still NULL or blank, so a value is never overwritten even if the Go-side
// check were bypassed.
func execAttrFill(ctx context.Context, tx *sql.Tx, w erpintegration.AttrFillWrite, fill erpintegration.AttrValues, now time.Time) (erpintegration.AttrValues, error) {
	actor := strings.TrimSpace(w.Actor)
	if actor == "" {
		actor = attrBackfillDefaultActor
	}
	var prd any
	if fill.PrdPerDay != "" {
		prd = fill.PrdPerDay
	}
	var after erpintegration.AttrValues
	err := tx.QueryRowContext(ctx, `
		UPDATE cost_product_master SET
			cpm_erp_fg_type = CASE WHEN NULLIF(TRIM(cpm_erp_fg_type),'') IS NULL
				THEN COALESCE(NULLIF($2,''), cpm_erp_fg_type) ELSE cpm_erp_fg_type END,
			cpm_erp_chp_item_code = CASE WHEN NULLIF(TRIM(cpm_erp_chp_item_code),'') IS NULL
				THEN COALESCE(NULLIF($3,''), cpm_erp_chp_item_code) ELSE cpm_erp_chp_item_code END,
			cpm_erp_ms_batch_item = CASE WHEN NULLIF(TRIM(cpm_erp_ms_batch_item),'') IS NULL
				THEN COALESCE(NULLIF($4,''), cpm_erp_ms_batch_item) ELSE cpm_erp_ms_batch_item END,
			cpm_erp_item_type = CASE WHEN NULLIF(TRIM(cpm_erp_item_type),'') IS NULL
				THEN COALESCE(NULLIF($5,''), cpm_erp_item_type) ELSE cpm_erp_item_type END,
			cpm_erp_prd_per_day = COALESCE(cpm_erp_prd_per_day, $6::numeric),
			cpm_updated_at = $7, cpm_updated_by = $8
		WHERE cpm_product_sys_id = $1
		RETURNING`+cpmAttrColumns,
		w.ProductSysID, fill.FgType, fill.ChpItemCode, fill.MsBatchItem, fill.ItemType, prd, now, actor).
		Scan(&after.FgType, &after.ChpItemCode, &after.MsBatchItem, &after.ItemType, &after.PrdPerDay)
	if err != nil {
		return after, fmt.Errorf("fill ERP attributes: %w", err)
	}
	return after, nil
}
