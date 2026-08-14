// Package mbhead provides application layer handlers for MB Head operations.
package mbhead

import (
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// mbHeadRowData holds parsed row values. Fields are populated by header name via
// mbHeadImportColumns, so the order here carries no meaning.
type mbHeadRowData struct {
	mbCosting    string
	mgtName      string
	devCode      string
	vsNumber     string
	noOfProcess  string
	shadeCode    string
	shadeName    string
	denier       string
	filament     string
	crossSection string
	ldrPrsn      string
	finalProduct string
	dozing       string
	lustureCode  string
	isBoughtout  string
	shadeCode2   string
	shadeName2   string
	shadeCode3   string
	shadeName3   string
}

// mbHeadExportRow is one entity plus its hydrated additional shades, the unit the export
// accessors read from. Shades are passed alongside rather than through the entity because
// ListAll does not hydrate children.
type mbHeadExportRow struct {
	entity *mbhead.Entity
	shades []*mbhead.Shade
}

// shadeBySeq returns the additional shade with the given sequence number, or nil.
func (r mbHeadExportRow) shadeBySeq(seq int32) *mbhead.Shade {
	for _, s := range r.shades {
		if s != nil && s.SeqNo() == seq {
			return s
		}
	}
	return nil
}

// shadeCodeBySeq returns the shade code for a sequence number, or "" when absent.
func (r mbHeadExportRow) shadeCodeBySeq(seq int32) any {
	if s := r.shadeBySeq(seq); s != nil {
		return s.ShadeCode()
	}
	return ""
}

// shadeNameBySeq returns the shade name for a sequence number, or "" when absent.
func (r mbHeadExportRow) shadeNameBySeq(seq int32) any {
	if s := r.shadeBySeq(seq); s != nil {
		return s.ShadeName()
	}
	return ""
}

// mbHeadImportColumn declares one recognized import column. Headers are matched by name,
// never by position, so the sheet's column order is irrelevant and unknown extra columns
// (such as the audit columns the export appends) are ignored.
//
// This one table is the single source of truth shared by the import parser, the export writer
// and the template generator: adding a column here adds it to all three, so the three can no
// longer drift apart and break round-tripping (defect I2).
type mbHeadImportColumn struct {
	// header is the canonical header text, as written by the template and the export.
	header string
	// required marks a column of spec section 2.1: its header must be present in the sheet
	// (a missing one aborts the whole file) and its cell must be non-empty on every data row
	// (an empty one fails that row only).
	required bool
	// assign copies the trimmed cell text into the parsed row.
	assign func(data *mbHeadRowData, value string)
	// export renders this column's cell for one exported entity.
	export func(row mbHeadExportRow) any
	// samples are the two illustrative template rows for this column.
	samples [2]string
}

// mbHeadImportColumns is the canonical column set (spec section 5.2), in column order.
// The first 12 are the required fields of spec section 2.1; the remainder are optional.
var mbHeadImportColumns = []mbHeadImportColumn{
	{
		header: "MB Costing", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.mbCosting = v },
		export:  func(r mbHeadExportRow) any { return r.entity.MBCosting() },
		samples: [2]string{"MBC-0001", "MBC-0002"},
	},
	{
		header: "MB Name", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.mgtName = v },
		export:  func(r mbHeadExportRow) any { return optStr(r.entity.MgtName()) },
		samples: [2]string{"Black MB Batch", "White MB Batch"},
	},
	{
		header: "Development No", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.devCode = v },
		export:  func(r mbHeadExportRow) any { return r.entity.DevCode() },
		samples: [2]string{"DEV-001", "DEV-002"},
	},
	{
		header: "VS Number", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.vsNumber = v },
		export:  func(r mbHeadExportRow) any { return r.entity.VsNumber() },
		samples: [2]string{"VS-0001", "VS-0002"},
	},
	{
		header: "No of Process", required: true,
		assign: func(d *mbHeadRowData, v string) { d.noOfProcess = v },
		export: func(r mbHeadExportRow) any { return r.entity.NoOfProcess() },
		// Deliberately blank: the permitted codes live in mst_mb_param_option and are never
		// hardcoded anywhere in this service. See the Instructions sheet.
		samples: [2]string{"", ""},
	},
	{
		header: "Shade Code", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.shadeCode = v },
		export:  func(r mbHeadExportRow) any { return r.entity.ShadeCode() },
		samples: [2]string{"SH-BLK", "SH-WHT"},
	},
	{
		header: "Shade Name", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.shadeName = v },
		export:  func(r mbHeadExportRow) any { return r.entity.ShadeName() },
		samples: [2]string{"Black", "White"},
	},
	{
		header: "POY Denier", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.denier = v },
		export:  func(r mbHeadExportRow) any { return optFloat(r.entity.Denier()) },
		samples: [2]string{"150", "75"},
	},
	{
		header: "POY Filament", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.filament = v },
		export:  func(r mbHeadExportRow) any { return optInt(r.entity.Filament()) },
		samples: [2]string{"48", "36"},
	},
	{
		header: "Cross Section", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.crossSection = v },
		export:  func(r mbHeadExportRow) any { return r.entity.CrossSection() },
		samples: [2]string{"ROUND", "ROUND"},
	},
	{
		header: "LDR %", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.ldrPrsn = v },
		export:  func(r mbHeadExportRow) any { return optFloat(r.entity.MBHLdrPrsn()) },
		samples: [2]string{"12.5", "8"},
	},
	{
		header: "Final Product", required: true,
		assign:  func(d *mbHeadRowData, v string) { d.finalProduct = v },
		export:  func(r mbHeadExportRow) any { return optStr(r.entity.MBHFinalProduct()) },
		samples: [2]string{"YARN", "YARN"},
	},
	{
		header:  "Dozing",
		assign:  func(d *mbHeadRowData, v string) { d.dozing = v },
		export:  func(r mbHeadExportRow) any { return optFloat(r.entity.Dozing()) },
		samples: [2]string{"1.2", "1.0"},
	},
	{
		header:  "Lusture Code",
		assign:  func(d *mbHeadRowData, v string) { d.lustureCode = v },
		export:  func(r mbHeadExportRow) any { return r.entity.LustureCode() },
		samples: [2]string{"LC-01", "LC-02"},
	},
	{
		header:  "Is Bought Out",
		assign:  func(d *mbHeadRowData, v string) { d.isBoughtout = v },
		export:  func(r mbHeadExportRow) any { return r.entity.IsBoughtout() },
		samples: [2]string{"FALSE", "TRUE"},
	},
	{
		header:  "Shade Code 2",
		assign:  func(d *mbHeadRowData, v string) { d.shadeCode2 = v },
		export:  func(r mbHeadExportRow) any { return r.shadeCodeBySeq(mbhead.MinShadeSeqNo) },
		samples: [2]string{"SH-BLK-2", ""},
	},
	{
		header:  "Shade Name 2",
		assign:  func(d *mbHeadRowData, v string) { d.shadeName2 = v },
		export:  func(r mbHeadExportRow) any { return r.shadeNameBySeq(mbhead.MinShadeSeqNo) },
		samples: [2]string{"Black Alt", ""},
	},
	{
		header:  "Shade Code 3",
		assign:  func(d *mbHeadRowData, v string) { d.shadeCode3 = v },
		export:  func(r mbHeadExportRow) any { return r.shadeCodeBySeq(mbhead.MaxShadeSeqNo) },
		samples: [2]string{"", ""},
	},
	{
		header:  "Shade Name 3",
		assign:  func(d *mbHeadRowData, v string) { d.shadeName3 = v },
		export:  func(r mbHeadExportRow) any { return r.shadeNameBySeq(mbhead.MaxShadeSeqNo) },
		samples: [2]string{"", ""},
	},
}

// mbHeadAuditExportColumns are appended to the export only. They are read-only provenance and
// are deliberately absent from mbHeadImportColumns, so re-importing an export ignores them.
var mbHeadAuditExportColumns = []mbHeadImportColumn{
	{
		header: "Active",
		export: func(r mbHeadExportRow) any { return r.entity.IsActive() },
	},
	{
		header: "Created At",
		export: func(r mbHeadExportRow) any { return r.entity.CreatedAt().Format("2006-01-02 15:04:05") },
	},
	{
		header: "Created By",
		export: func(r mbHeadExportRow) any { return r.entity.CreatedBy() },
	},
}

// mbHeadExportColumns is the full export column order: the canonical round-trippable set
// followed by the audit columns. The legacy leading "No" column is gone — it was the reason
// an export could not be fed straight back into the import (defect I2).
func mbHeadExportColumns() []mbHeadImportColumn {
	cols := make([]mbHeadImportColumn, 0, len(mbHeadImportColumns)+len(mbHeadAuditExportColumns))
	cols = append(cols, mbHeadImportColumns...)
	cols = append(cols, mbHeadAuditExportColumns...)
	return cols
}

// mbHeadColumnHeaders returns the header texts of cols, in order.
func mbHeadColumnHeaders(cols []mbHeadImportColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.header
	}
	return out
}

// mbHeadColumnIndex maps each recognized column to the zero-based sheet column it was found in.
// A column absent from the sheet is absent from the map and parses as an empty value.
type mbHeadColumnIndex map[string]int

// normalizeHeader canonicalizes a header cell for matching: BOM stripped, surrounding and
// repeated internal whitespace collapsed to single spaces, case folded. These files are
// hand-edited, so "  mb costing " and "MB  Costing" must both match "MB Costing".
func normalizeHeader(s string) string {
	s = strings.TrimPrefix(s, "\ufeff")
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// resolveMBHeadColumns builds the header to column map from the sheet's first row. A missing
// required header aborts the whole file rather than failing every row individually.
func resolveMBHeadColumns(headerRow []string) (mbHeadColumnIndex, error) {
	seen := make(map[string]int, len(headerRow))
	for i, cell := range headerRow {
		key := normalizeHeader(cell)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue // first occurrence wins
		}
		seen[key] = i
	}

	cols := make(mbHeadColumnIndex, len(mbHeadImportColumns))
	var missing []string
	for _, col := range mbHeadImportColumns {
		idx, ok := seen[normalizeHeader(col.header)]
		if !ok {
			if col.required {
				missing = append(missing, col.header)
			}
			continue
		}
		cols[col.header] = idx
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required column(s) in header row: %s", strings.Join(missing, ", "))
	}

	return cols, nil
}

// parseMBHeadRow maps a data row into mbHeadRowData using the resolved header positions.
func parseMBHeadRow(row []string, cols mbHeadColumnIndex) mbHeadRowData {
	var data mbHeadRowData
	for _, col := range mbHeadImportColumns {
		idx, ok := cols[col.header]
		if !ok {
			continue
		}
		col.assign(&data, getCell(row, idx))
	}
	return data
}

// getCell safely gets a cell value from a row.
func getCell(row []string, index int) string {
	if index >= 0 && index < len(row) {
		return strings.TrimSpace(row[index])
	}
	return ""
}
