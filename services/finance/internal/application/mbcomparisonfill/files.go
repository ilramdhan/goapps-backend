package mbcomparisonfill

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mblegacyfill"
)

// Column indexes of dozing.csv (column 0 = Excel row; header = first record).
const (
	colItem, colShade, colCode, colDye, colDen, colFil, colCC = 3, 5, 7, 8, 9, 10, 11
	colSPDoz, colConv, colRPDoz, colSysID                     = 12, 13, 15, 16
	minCols                                                   = 17
)

var cellParams = map[int]string{
	colCode: "MB_SP_CODE", colDye: "MB_SP_DYE", colDen: "MB_SP_DENIER", colFil: "MB_SP_FILAMENT", colCC: "MB_SP_CC",
	colSPDoz: "MB_SP_DOZING", colConv: "CONV_FACTOR_ACTUAL", colRPDoz: "RP_DOZING_ACTUAL",
}

func readCSV(path string) (recs [][]string, err error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied input path for a one-shot CLI
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, cErr)
		}
	}()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord, cr.LazyQuotes = -1, true
	recs, err = cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(recs) > 0 && len(recs[0]) > 0 {
		recs[0][0] = strings.TrimPrefix(recs[0][0], "\xef\xbb\xbf")
	}
	return recs, nil
}

// ParseExcel reads the Excel dump (first record is the header).
func ParseExcel(path string) ([]ExcelRow, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	out := make([]ExcelRow, 0, len(recs))
	for i, rec := range recs {
		if i == 0 || len(rec) < minCols {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			continue
		}
		out = append(out, ParseRow(n, rec))
	}
	return out, nil
}

// ParseRow builds an ExcelRow from one record (>= minCols columns).
func ParseRow(n int, rec []string) ExcelRow {
	r := ExcelRow{Row: n, ItemCode: strings.TrimSpace(rec[colItem]), Shade: strings.TrimSpace(rec[colShade]),
		SysID: strings.TrimSpace(rec[colSysID]), Cells: map[string]string{}}
	for col, code := range cellParams {
		c := strings.TrimSpace(rec[col])
		if strings.EqualFold(c, "#N/A") {
			r.HasNA = true
		}
		k, _ := ParamKind(code)
		if k == mblegacyfill.KindNum && c != "" && !r.HasNA {
			if _, ok := NormNum(c); !ok {
				r.Malformed = code + "=" + c
			}
		}
		r.Cells[code] = c
	}
	// Finance's lookup check columns (17+) carry #N/A for MBs missing in their lookup (24 rows) -> REPORT.
	for _, c := range append([]string{rec[colItem], rec[colShade]}, rec[minCols:]...) {
		if strings.EqualFold(strings.TrimSpace(c), "#N/A") {
			r.HasNA = true
		}
	}
	return r
}

// ParseOrion reads the Oracle head dump (O-01b): SYSID (CMBH_SYS_ID) -> CMBH_ORION_ITEM_CODE; blanks omitted.
func ParseOrion(path string) (map[string]string, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, errors.New("orion csv is empty")
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		idx[strings.TrimSpace(h)] = i
	}
	si, ok1 := idx["CMBH_SYS_ID"]
	oi, ok2 := idx["CMBH_ORION_ITEM_CODE"]
	if !ok1 || !ok2 {
		return nil, errors.New("orion csv lacks CMBH_SYS_ID / CMBH_ORION_ITEM_CODE")
	}
	m := map[string]string{}
	for _, rec := range recs[1:] {
		if len(rec) <= si || len(rec) <= oi {
			continue
		}
		if v := strings.TrimSpace(rec[oi]); v != "" {
			m[strings.TrimSpace(rec[si])] = v
		}
	}
	return m, nil
}

// PlanHeader is the plan CSV header.
var PlanHeader = []string{"excel_row", "item_code", "shade", "product_sys_id", "decision", "reason", "before_json", "after_json", "capp_json", "before_existed_json", "spin_ptr_cleared"}

// WritePlanCSV writes the plan rows.
func WritePlanCSV(w io.Writer, plan *Plan) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(PlanHeader); err != nil {
		return fmt.Errorf("write plan header: %w", err)
	}
	for _, r := range plan.Rows {
		if err := cw.Write(planRecord(r)); err != nil {
			return fmt.Errorf("write plan row: %w", err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// planRecord renders one row in PlanHeader order (shared by the plan and result CSVs).
func planRecord(r PlanRow) []string {
	return []string{strconv.Itoa(r.ExcelRow), r.ItemCode, r.Shade, strconv.FormatInt(r.ProductID, 10),
		string(r.Decision), r.Reason, mustJSON(r.Before), mustJSON(r.After), mustJSON(r.CAPP), mustJSON(r.BeforeExisted), r.SpinPtr}
}

// WriteSummaryCSV writes section,key,count lines.
func WriteSummaryCSV(w io.Writer, plan *Plan) error {
	cw := csv.NewWriter(w)
	recs := [][]string{{"section", "key", "count"}, {"plan", "rows", strconv.Itoa(len(plan.Rows))}}
	c := plan.Counts()
	for _, d := range []Decision{DecisionApply, DecisionSkipSame, DecisionReview, DecisionReport} {
		recs = append(recs, []string{"decision", string(d), strconv.Itoa(c[d])})
	}
	reasons := map[string]int{}
	for _, r := range plan.Rows {
		if r.Decision == DecisionReport || r.Decision == DecisionReview {
			reasons[string(r.Decision)+"/"+strings.SplitN(r.Reason, "(", 2)[0]]++
		}
	}
	for k, v := range reasons {
		recs = append(recs, []string{"reason", k, strconv.Itoa(v)})
	}
	recs = append(recs, []string{"capp", "products_with_cpp_but_no_capp", strconv.Itoa(plan.CPPWithoutCAPP)},
		[]string{"spin_ptr", "products_with_spin_ptr_cleared", strconv.Itoa(plan.SpinPtrCleared)})
	if err := cw.WriteAll(recs); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return cw.Error()
}

// ReadPlanCSV loads a plan CSV back (all decisions).
func ReadPlanCSV(path string) ([]PlanRow, error) {
	recs, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 || strings.Join(recs[0], ",") != strings.Join(PlanHeader, ",") {
		return nil, errors.New("not a mb-comparison-fill plan csv (header mismatch)")
	}
	out := make([]PlanRow, 0, len(recs))
	for _, rec := range recs[1:] {
		if len(rec) != len(PlanHeader) {
			return nil, fmt.Errorf("plan row has %d columns, want %d", len(rec), len(PlanHeader))
		}
		pr, err := ReadPlanCSVRecord(rec)
		if err != nil {
			return nil, err
		}
		out = append(out, pr)
	}
	return out, nil
}

// ReadPlanCSVRecord parses one plan record.
func ReadPlanCSVRecord(rec []string) (PlanRow, error) {
	var pr PlanRow
	var err error
	if pr.ExcelRow, err = strconv.Atoi(rec[0]); err != nil {
		return pr, fmt.Errorf("excel_row %q: %w", rec[0], err)
	}
	if pr.ProductID, err = strconv.ParseInt(rec[3], 10, 64); err != nil {
		return pr, fmt.Errorf("product_sys_id %q: %w", rec[3], err)
	}
	pr.ItemCode, pr.Shade, pr.Decision, pr.Reason, pr.SpinPtr = rec[1], rec[2], Decision(rec[4]), rec[5], rec[10]
	if err = unmarshalAll(rec[6:10], &pr.Before, &pr.After, &pr.CAPP, &pr.BeforeExisted); err != nil {
		return pr, fmt.Errorf("excel_row %d: %w", pr.ExcelRow, err)
	}
	return pr, nil
}

func unmarshalAll(src []string, dst ...any) error {
	for i, s := range src {
		if err := json.Unmarshal([]byte(s), dst[i]); err != nil {
			return fmt.Errorf("json column %d: %w", i, err)
		}
	}
	return nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null" // unreachable for map[string]string / []string
	}
	return string(b)
}
