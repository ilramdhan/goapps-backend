package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// ErpStdCostRepository is the batch-scoped store of
// cst_erp_std_cost (migration 000549; design §4.2; plan-05 P4-T4).
//
//   - Replace is DELETE + chunked INSERT of the batch's rows in one PG
//     transaction; only rows of that batch are touched.
//   - Digest recomputes the batch control digest (erpintegration.StdDigest)
//     from the stored rows in SQL, with the same canonical text as
//     erpintegration.ComputeStdDigest.
type ErpStdCostRepository struct{ db *DB }

// NewErpStdCostRepository constructs the repository.
func NewErpStdCostRepository(db *DB) *ErpStdCostRepository {
	return &ErpStdCostRepository{db: db}
}

// erpStdInsertChunk bounds one INSERT statement: ~40 parallel arrays of 1000
// elements keeps the bind payload a few MB even at ~3.7 KB/row (P4-T5).
const erpStdInsertChunk = 1000

// Replace deletes and re-inserts the batch's std rows in one transaction.
func (r *ErpStdCostRepository) Replace(ctx context.Context, batchID int64, period string, rows []erpintegration.StdRow) (int64, error) {
	var n int64
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = replaceStdRows(ctx, tx, batchID, period, rows)
		return err
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// List returns the batch's std rows ordered by (item, grade, shade).
func (r *ErpStdCostRepository) List(ctx context.Context, batchID int64) ([]erpintegration.StdRow, error) {
	return listStdRows(ctx, r.db, batchID)
}

// Digest recomputes the control digest from the stored rows.
func (r *ErpStdCostRepository) Digest(ctx context.Context, batchID int64) (erpintegration.StdDigest, error) {
	return stdDigest(ctx, r.db, batchID)
}

const erpStdDeleteSQL = `DELETE FROM cst_erp_std_cost WHERE cesc_batch_id = $1`

// erpStdInsertSQL inserts one chunk from parallel arrays. Decimals travel as
// text and are cast to NUMERIC server-side (no float anywhere). The ERP
// read-back / recon columns (cesc_prev_std_cost, cesc_erp_*, cesc_recon_status)
// belong to later steps and start NULL.
const erpStdInsertSQL = `
	INSERT INTO cst_erp_std_cost (
		cesc_batch_id, cesc_period,
		cesc_item_code, cesc_grade_code, cesc_shade_code, cesc_item_name, cesc_shade_name,
		cesc_item_kind, cesc_source, cesc_ax_cost_sys_id, cesc_ax_cost_version, cesc_product_sys_id,
		cesc_prod_type, cesc_grade_group, cesc_fg_type, cesc_basis, cesc_chp_item_code,
		cesc_chp_con_kg, cesc_chp_cost, cesc_ax_conv_cost, cesc_conv_cost,
		cesc_conv_cost1, cesc_conv_cost2, cesc_conv_cost4, cesc_conv_cost5,
		cesc_selling_price, cesc_value_loss, cesc_ax_cost, cesc_std_cost, cesc_prod_value_loss,
		cesc_ms_batch_item, cesc_item_type, cesc_prd_per_day, cesc_status, cesc_validation)
	SELECT $1, $2,
	       u.item, u.grade, u.shade, u.item_name, u.shade_name,
	       u.kind, u.source, u.ax_id, u.ax_ver, u.prod_id,
	       u.prod_type, u.grade_group, u.fg_type, u.basis, u.chp_item,
	       u.chp_kg, u.chp_cost, u.ax_conv, u.conv,
	       u.conv1, u.conv2, u.conv4, u.conv5,
	       u.sell, u.vloss, u.ax_cost, u.std, u.pvl,
	       u.ms_batch, u.item_type, u.prd_day, u.status, u.validation
	  FROM unnest(
		$3::text[], $4::text[], $5::text[], $6::text[], $7::text[],
		$8::text[], $9::text[], $10::bigint[], $11::int[], $12::bigint[],
		$13::text[], $14::text[], $15::text[], $16::text[], $17::text[],
		$18::numeric[], $19::numeric[], $20::numeric[], $21::numeric[],
		$22::numeric[], $23::numeric[], $24::numeric[], $25::numeric[],
		$26::numeric[], $27::numeric[], $28::numeric[], $29::numeric[], $30::numeric[],
		$31::text[], $32::text[], $33::numeric[], $34::text[], $35::jsonb[]
	  ) AS u(item, grade, shade, item_name, shade_name,
	         kind, source, ax_id, ax_ver, prod_id,
	         prod_type, grade_group, fg_type, basis, chp_item,
	         chp_kg, chp_cost, ax_conv, conv,
	         conv1, conv2, conv4, conv5,
	         sell, vloss, ax_cost, std, pvl,
	         ms_batch, item_type, prd_day, status, validation)`

// replaceStdRows is the tx-scoped DELETE + chunked INSERT.
func replaceStdRows(ctx context.Context, q erpQuerier, batchID int64, period string, rows []erpintegration.StdRow) (int64, error) {
	if batchID <= 0 {
		return 0, fmt.Errorf("erp std replace: invalid batch id %d", batchID)
	}
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return 0, fmt.Errorf("erp std replace: %w", err)
	}
	for i := range rows {
		if err := checkStdRow(rows[i]); err != nil {
			return 0, fmt.Errorf("erp std replace row %d: %w", i, err)
		}
	}
	if _, err := q.ExecContext(ctx, erpStdDeleteSQL, batchID); err != nil {
		return 0, fmt.Errorf("erp std delete: %w", err)
	}
	var n int64
	for start := 0; start < len(rows); start += erpStdInsertChunk {
		end := min(start+erpStdInsertChunk, len(rows))
		args, err := stdInsertArgs(batchID, period, rows[start:end])
		if err != nil {
			return 0, err
		}
		res, err := q.ExecContext(ctx, erpStdInsertSQL, args...)
		if err != nil {
			return 0, fmt.Errorf("erp std insert rows %d..%d: %w", start, end-1, err)
		}
		c, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("erp std rows affected: %w", err)
		}
		n += c
	}
	return n, nil
}

func checkStdRow(r erpintegration.StdRow) error {
	if r.Key.ItemCode == "" || r.Key.GradeCode == "" {
		return fmt.Errorf("empty key %q", r.Key.String())
	}
	if _, err := erpintegration.ParseItemKind(string(r.Kind)); err != nil {
		return err
	}
	if _, err := erpintegration.ParseDeriveStatus(string(r.Status)); err != nil {
		return err
	}
	return nil
}

// stdIssueJSON is the stored form of one erpintegration.Issue in
// cesc_validation (the key is the row's own key and is not repeated).
type stdIssueJSON struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func issuesJSON(issues []erpintegration.Issue) (string, error) {
	out := make([]stdIssueJSON, len(issues))
	for i, is := range issues {
		out[i] = stdIssueJSON{Code: string(is.Code), Severity: string(is.Severity), Message: is.Message}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("erp std validation json: %w", err)
	}
	return string(b), nil
}

// stdInsertCols holds the parallel arrays of one insert chunk.
type stdInsertCols struct {
	item, grade, shade, kind, status, validation                     []string
	itemName, shadeName, source, prodType, gradeGroup, fgType, basis []sql.NullString
	chpItem, msBatch, itemType                                       []sql.NullString
	axID, prodID                                                     []sql.NullInt64
	axVer                                                            []sql.NullInt32
	chpKg, chpCost, axConv, conv, conv1, conv2, conv4, conv5         []sql.NullString
	sell, vloss, axCost, std, pvl, prdDay                            []sql.NullString
}

func newStdInsertCols(c int) *stdInsertCols {
	s := func() []string { return make([]string, c) }
	ns := func() []sql.NullString { return make([]sql.NullString, c) }
	return &stdInsertCols{
		item: s(), grade: s(), shade: s(), kind: s(), status: s(), validation: s(),
		itemName: ns(), shadeName: ns(), source: ns(), prodType: ns(), gradeGroup: ns(), fgType: ns(), basis: ns(),
		chpItem: ns(), msBatch: ns(), itemType: ns(),
		axID: make([]sql.NullInt64, c), prodID: make([]sql.NullInt64, c), axVer: make([]sql.NullInt32, c),
		chpKg: ns(), chpCost: ns(), axConv: ns(), conv: ns(), conv1: ns(), conv2: ns(), conv4: ns(), conv5: ns(),
		sell: ns(), vloss: ns(), axCost: ns(), std: ns(), pvl: ns(), prdDay: ns(),
	}
}

func (c *stdInsertCols) set(i int, r *erpintegration.StdRow) error {
	c.item[i], c.grade[i], c.shade[i] = r.Key.ItemCode, r.Key.GradeCode, r.Key.ShadeCode
	c.kind[i], c.status[i] = string(r.Kind), string(r.Status)
	v, err := issuesJSON(r.Issues)
	if err != nil {
		return err
	}
	c.validation[i] = v
	c.itemName[i], c.shadeName[i] = nullString(r.ItemName), nullString(r.ShadeName)
	c.source[i], c.prodType[i] = nullString(string(r.Source)), nullString(string(r.ProdType))
	c.gradeGroup[i], c.fgType[i], c.basis[i] = nullString(string(r.GradeGroup)), nullString(r.FgType), nullString(string(r.Basis))
	c.chpItem[i], c.msBatch[i], c.itemType[i] = nullString(r.ChpItemCode), nullString(r.MsBatchItem), nullString(r.ItemType)
	c.axID[i], c.prodID[i], c.axVer[i] = erpNullInt64(r.AxCostSysID), erpNullInt64(r.ProductSysID), erpNullInt32(r.AxCostVersion)
	for _, p := range []struct {
		dst *sql.NullString
		src decimal.NullDecimal
	}{
		{&c.chpKg[i], r.ChpConKg}, {&c.chpCost[i], r.ChpCost}, {&c.axConv[i], r.AxConvCost},
		{&c.conv[i], r.ConvCost}, {&c.conv1[i], r.ConvCost1}, {&c.conv2[i], r.ConvCost2},
		{&c.conv4[i], r.ConvCost4}, {&c.conv5[i], r.ConvCost5}, {&c.sell[i], r.SellingPrice},
		{&c.vloss[i], r.ValueLoss}, {&c.axCost[i], r.AxCost}, {&c.std[i], r.StdCost},
		{&c.pvl[i], r.ProdValLoss}, {&c.prdDay[i], r.PrdPerDay},
	} {
		*p.dst = nullDecimalText(p.src)
	}
	return nil
}

func stdInsertArgs(batchID int64, period string, rows []erpintegration.StdRow) ([]any, error) {
	c := newStdInsertCols(len(rows))
	for i := range rows {
		if err := c.set(i, &rows[i]); err != nil {
			return nil, err
		}
	}
	return []any{
		batchID, period,
		pq.Array(c.item), pq.Array(c.grade), pq.Array(c.shade), pq.Array(c.itemName), pq.Array(c.shadeName),
		pq.Array(c.kind), pq.Array(c.source), pq.Array(c.axID), pq.Array(c.axVer), pq.Array(c.prodID),
		pq.Array(c.prodType), pq.Array(c.gradeGroup), pq.Array(c.fgType), pq.Array(c.basis), pq.Array(c.chpItem),
		pq.Array(c.chpKg), pq.Array(c.chpCost), pq.Array(c.axConv), pq.Array(c.conv),
		pq.Array(c.conv1), pq.Array(c.conv2), pq.Array(c.conv4), pq.Array(c.conv5),
		pq.Array(c.sell), pq.Array(c.vloss), pq.Array(c.axCost), pq.Array(c.std), pq.Array(c.pvl),
		pq.Array(c.msBatch), pq.Array(c.itemType), pq.Array(c.prdDay), pq.Array(c.status), pq.Array(c.validation),
	}, nil
}

const erpStdListSQL = `
	SELECT cesc_item_code, cesc_grade_code, cesc_shade_code,
	       COALESCE(cesc_item_name, ''), COALESCE(cesc_shade_name, ''), cesc_item_kind,
	       COALESCE(cesc_source, ''), cesc_ax_cost_sys_id, cesc_ax_cost_version, cesc_product_sys_id,
	       COALESCE(cesc_prod_type, ''), COALESCE(cesc_grade_group, ''), COALESCE(cesc_fg_type, ''),
	       COALESCE(cesc_basis, ''), COALESCE(cesc_chp_item_code, ''),
	       cesc_chp_con_kg::text, cesc_chp_cost::text, cesc_ax_conv_cost::text, cesc_conv_cost::text,
	       cesc_conv_cost1::text, cesc_conv_cost2::text, cesc_conv_cost4::text, cesc_conv_cost5::text,
	       cesc_selling_price::text, cesc_value_loss::text, cesc_ax_cost::text, cesc_std_cost::text,
	       cesc_prod_value_loss::text,
	       COALESCE(cesc_ms_batch_item, ''), COALESCE(cesc_item_type, ''), cesc_prd_per_day::text,
	       cesc_status, cesc_validation::text
	  FROM cst_erp_std_cost
	 WHERE cesc_batch_id = $1
	 ORDER BY cesc_item_code COLLATE "C", cesc_grade_code COLLATE "C", cesc_shade_code COLLATE "C"`

func listStdRows(ctx context.Context, q erpQuerier, batchID int64) (out []erpintegration.StdRow, err error) {
	rows, err := q.QueryContext(ctx, erpStdListSQL, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp std list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp std list")
	for rows.Next() {
		r, err := scanStdRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp std list rows: %w", err)
	}
	return out, nil
}

func scanStdRow(s rowScanner) (erpintegration.StdRow, error) {
	var (
		r                                            erpintegration.StdRow
		kind, source, prodType, gradeGroup, basis    string
		status, validation                           string
		axID, prodID                                 sql.NullInt64
		axVer                                        sql.NullInt32
		chpKg, chpCost, axConv, conv, c1, c2, c4, c5 sql.NullString
		sell, vloss, axCost, std, pvl, prdDay        sql.NullString
	)
	if err := s.Scan(&r.Key.ItemCode, &r.Key.GradeCode, &r.Key.ShadeCode,
		&r.ItemName, &r.ShadeName, &kind,
		&source, &axID, &axVer, &prodID,
		&prodType, &gradeGroup, &r.FgType, &basis, &r.ChpItemCode,
		&chpKg, &chpCost, &axConv, &conv, &c1, &c2, &c4, &c5,
		&sell, &vloss, &axCost, &std, &pvl,
		&r.MsBatchItem, &r.ItemType, &prdDay,
		&status, &validation); err != nil {
		return erpintegration.StdRow{}, fmt.Errorf("erp std scan: %w", err)
	}
	k, err := erpintegration.ParseItemKind(kind)
	if err != nil {
		return erpintegration.StdRow{}, err
	}
	st, err := erpintegration.ParseDeriveStatus(status)
	if err != nil {
		return erpintegration.StdRow{}, err
	}
	r.Kind, r.Status = k, st
	r.Source = erpintegration.StdSource(source)
	r.ProdType, r.GradeGroup, r.Basis = erprule.ProdType(prodType), erprule.GradeGroup(gradeGroup), erprule.Basis(basis)
	if axID.Valid {
		v := axID.Int64
		r.AxCostSysID = &v
	}
	if axVer.Valid {
		v := axVer.Int32
		r.AxCostVersion = &v
	}
	if prodID.Valid {
		v := prodID.Int64
		r.ProductSysID = &v
	}
	for _, p := range []struct {
		src sql.NullString
		dst *decimal.NullDecimal
	}{
		{chpKg, &r.ChpConKg}, {chpCost, &r.ChpCost}, {axConv, &r.AxConvCost}, {conv, &r.ConvCost},
		{c1, &r.ConvCost1}, {c2, &r.ConvCost2}, {c4, &r.ConvCost4}, {c5, &r.ConvCost5},
		{sell, &r.SellingPrice}, {vloss, &r.ValueLoss}, {axCost, &r.AxCost}, {std, &r.StdCost},
		{pvl, &r.ProdValLoss}, {prdDay, &r.PrdPerDay},
	} {
		if *p.dst, err = parseNullDecimalText(p.src); err != nil {
			return erpintegration.StdRow{}, fmt.Errorf("erp std decimal %s: %w", r.Key.String(), err)
		}
	}
	var issues []stdIssueJSON
	if err := json.Unmarshal([]byte(validation), &issues); err != nil {
		return erpintegration.StdRow{}, fmt.Errorf("erp std validation %s: %w", r.Key.String(), err)
	}
	for _, is := range issues {
		r.Issues = append(r.Issues, erpintegration.Issue{
			Key: r.Key, Code: erpintegration.IssueCode(is.Code),
			Severity: erpintegration.Severity(is.Severity), Message: is.Message,
		})
	}
	return r, nil
}

// erpStdDigestSQL is the SQL twin of erpintegration.ComputeStdDigest: OK rows
// only, byte-wise (COLLATE "C") key order, "|"-joined fields with NULL as ”
// (concat_ws alone would skip NULLs), NUMERIC(20,5)::text = fixed 5 dp.
const erpStdDigestSQL = `
	SELECT count(*),
	       COALESCE(sum(cesc_std_cost), 0)::text,
	       COALESCE(sum(cesc_conv_cost), 0)::text,
	       COALESCE(sum(cesc_prod_value_loss), 0)::text,
	       md5(COALESCE(string_agg(concat_ws('|',
	           cesc_item_code, cesc_grade_code, cesc_shade_code,
	           COALESCE(cesc_source, ''), COALESCE(cesc_basis, ''), COALESCE(cesc_fg_type, ''),
	           COALESCE(cesc_chp_cost::text, ''), COALESCE(cesc_ax_conv_cost::text, ''),
	           COALESCE(cesc_conv_cost::text, ''), COALESCE(cesc_selling_price::text, ''),
	           COALESCE(cesc_value_loss::text, ''), COALESCE(cesc_ax_cost::text, ''),
	           COALESCE(cesc_std_cost::text, ''), COALESCE(cesc_prod_value_loss::text, '')),
	         E'\n' ORDER BY cesc_item_code COLLATE "C", cesc_grade_code COLLATE "C", cesc_shade_code COLLATE "C"), ''))
	  FROM cst_erp_std_cost
	 WHERE cesc_batch_id = $1 AND cesc_status = 'OK'`

func stdDigest(ctx context.Context, q erpQuerier, batchID int64) (erpintegration.StdDigest, error) {
	var (
		n                     int64
		sumStd, sumConv, sumP string
		md5                   string
	)
	if err := q.QueryRowContext(ctx, erpStdDigestSQL, batchID).Scan(&n, &sumStd, &sumConv, &sumP, &md5); err != nil {
		return erpintegration.StdDigest{}, fmt.Errorf("erp std digest: %w", err)
	}
	var sums [3]decimal.Decimal
	for i, s := range []string{sumStd, sumConv, sumP} {
		d, err := erpintegration.ParseDecimal(s)
		if err != nil {
			return erpintegration.StdDigest{}, fmt.Errorf("erp std digest sum: %w", err)
		}
		sums[i] = d
	}
	totals, err := erpintegration.NewControlTotals(n, sums[0], sums[1], sums[2])
	if err != nil {
		return erpintegration.StdDigest{}, err
	}
	return erpintegration.StdDigest{Totals: totals, RowsMD5: md5}, nil
}

const erpReconListSQL = `
	SELECT cesc_item_code, cesc_grade_code, cesc_shade_code, COALESCE(cesc_basis, ''), cesc_std_cost,
	       cesc_recon_status, cesc_erp_rate, cesc_erp_rate_variants, cesc_erp_qty_kg, cesc_erp_value,
	       COALESCE(cesc_erp_flex13, '')
	FROM cst_erp_std_cost
	WHERE cesc_batch_id = $1 AND cesc_recon_status IS NOT NULL
	ORDER BY cesc_item_code, cesc_grade_code, cesc_shade_code`

// ListRecon returns the recon-classified std rows of a batch (OK rows only).
func (r *ErpStdCostRepository) ListRecon(ctx context.Context, batchID int64) (out []erpintegration.ReconExportRow, err error) {
	rows, err := r.db.QueryContext(ctx, erpReconListSQL, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp recon list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp recon list")
	for rows.Next() {
		var (
			x                   erpintegration.ReconExportRow
			status              string
			std, rate, qty, val sql.NullString
			variants            sql.NullInt64
		)
		if err := rows.Scan(&x.Key.ItemCode, &x.Key.GradeCode, &x.Key.ShadeCode, &x.Basis, &std,
			&status, &rate, &variants, &qty, &val, &x.Flex13); err != nil {
			return nil, fmt.Errorf("erp recon scan: %w", err)
		}
		x.Status = erpintegration.ReconStatus(status)
		for _, p := range []struct {
			s   sql.NullString
			dst *decimal.NullDecimal
		}{{std, &x.StdCost}, {rate, &x.ErpRate}, {qty, &x.QtyKg}, {val, &x.Value}} {
			if p.s.Valid {
				d, derr := decimal.NewFromString(p.s.String)
				if derr != nil {
					return nil, fmt.Errorf("erp recon decimal: %w", derr)
				}
				*p.dst = decimal.NullDecimal{Decimal: d, Valid: true}
			}
		}
		if variants.Valid {
			v := variants.Int64
			x.RateVariants = &v
		}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp recon list rows: %w", err)
	}
	return out, nil
}
