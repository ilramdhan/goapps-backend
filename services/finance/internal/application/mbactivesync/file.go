package mbactivesync

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// FileRow is one data row of the Finance mb-active file.
type FileRow struct {
	Ln        int    // 1-based line number in the source file (header = line 1 in the original format)
	POY       string // item code (POY), trimmed; "" when blank
	POYName   string
	Shade     string // trimmed; "" when blank
	ShadeName string
	MBCode    string // trimmed; "" when blank
	DyeName   string
	DozRaw    string // original text of RP-Doz (trimmed)
	Doz       float64
	DozOK     bool // false when RP-Doz is blank
	SysID     string

	PType string   // first token of POYName (e.g. "POY")
	Den   *float64 // denier from POYName "POY 380/144/TBL..."; nil when unparsed
	Fil   *int     // filament count; nil when unparsed
	CS    string   // cross-section token ("" when absent)
}

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ParseFile reads either the normalized file (comma, no header, 9 columns:
// ln,poy,poyname,shade,shadename,mbcode,dyename,doz,sysid) or the original
// Finance file (';', optional UTF-8 BOM, CRLF, header, comma decimal).
func ParseFile(path string) ([]FileRow, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied CLI path
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseBytes(b)
}

// ParseBytes parses file content, auto-detecting the format.
func ParseBytes(b []byte) ([]FileRow, error) {
	b = bytes.TrimPrefix(b, utf8BOM)
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, errors.New("mb-active file is empty")
	}
	first := b
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		first = b[:i]
	}
	if bytes.Contains(first, []byte(";")) {
		return parseOriginal(b)
	}
	return parseNormalized(b)
}

func newReader(b []byte, comma rune) *csv.Reader {
	r := csv.NewReader(bytes.NewReader(b))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r
}

func parseNormalized(b []byte) ([]FileRow, error) {
	r := newReader(b, ',')
	var rows []FileRow
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse normalized csv: %w", err)
		}
		line, _ := r.FieldPos(0)
		if len(rec) != 9 {
			return nil, fmt.Errorf("line %d: want 9 columns, got %d", line, len(rec))
		}
		ln, err := strconv.Atoi(strings.TrimSpace(rec[0]))
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid ln %q", line, rec[0])
		}
		row, err := buildRow(ln, rec[1:])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseOriginal(b []byte) ([]FileRow, error) {
	r := newReader(b, ';')
	var rows []FileRow
	header := true
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse original csv: %w", err)
		}
		line, _ := r.FieldPos(0)
		if header {
			header = false
			if len(rec) != 8 || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(rec[0])), "item code") {
				return nil, fmt.Errorf("line %d: unexpected header %q", line, rec)
			}
			continue
		}
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue // trailing blank line
		}
		if len(rec) != 8 {
			return nil, fmt.Errorf("line %d: want 8 columns, got %d", line, len(rec))
		}
		row, err := buildRow(line, rec)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// buildRow takes the 8 data columns: poy, poyname, shade, shadename, mbcode, dyename, doz, sysid.
func buildRow(ln int, c []string) (FileRow, error) {
	row := FileRow{
		Ln:        ln,
		POY:       strings.TrimSpace(c[0]),
		POYName:   strings.TrimSpace(c[1]),
		Shade:     strings.TrimSpace(c[2]),
		ShadeName: strings.TrimSpace(c[3]),
		MBCode:    strings.TrimSpace(c[4]),
		DyeName:   strings.TrimSpace(c[5]),
		DozRaw:    strings.TrimSpace(c[6]),
		SysID:     strings.TrimSpace(c[7]),
	}
	if row.SysID == "" {
		return row, errors.New("blank SYSID")
	}
	if row.DozRaw != "" {
		v, err := strconv.ParseFloat(strings.ReplaceAll(row.DozRaw, ",", "."), 64)
		if err != nil {
			return row, fmt.Errorf("invalid RP-Doz %q", row.DozRaw)
		}
		row.Doz, row.DozOK = v, true
	}
	parseSpec(&row)
	return row, nil
}

// parseSpec ports L-00: POYName "POY 380/144/TBL/..." -> type, den, fil, cs.
func parseSpec(r *FileRow) {
	parts := strings.Split(r.POYName, " ")
	r.PType = parts[0]
	if len(parts) < 2 {
		return
	}
	seg := strings.Split(parts[1], "/")
	if v, err := strconv.ParseFloat(seg[0], 64); err == nil && isDigitsDot(seg[0]) {
		r.Den = &v
	}
	if len(seg) > 1 && isDigits(seg[1]) {
		if n, err := strconv.Atoi(seg[1]); err == nil {
			r.Fil = &n
		}
	}
	if len(seg) > 2 {
		r.CS = seg[2]
	}
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isDigitsDot(s string) bool {
	return s != "" && strings.Trim(s, "0123456789.") == ""
}

// DistinctSysIDs returns the number of distinct SYSIDs in rows.
func DistinctSysIDs(rows []FileRow) int {
	m := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		m[r.SysID] = struct{}{}
	}
	return len(m)
}
