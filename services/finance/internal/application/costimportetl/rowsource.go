package costimportetl

import (
	"errors"
	"strings"
)

// rowSource yields a RowProducer for a logical layer token, abstracting over the
// upload transport (a zip of CSVs vs an xlsx workbook). Close releases the
// underlying container (its spooled temp file or open workbook).
type rowSource interface {
	// produce returns a RowProducer that streams the rows for token. A token
	// with no matching entry/sheet yields zero rows (not an error), so an
	// optional sheet that is simply absent resolves to an empty layer.
	produce(token string) RowProducer
	// Close releases the source's resources.
	Close() error
}

// zipSource streams CSV entries out of a spooled zip container, matching the
// entry name by case-insensitive substring on the layer token.
type zipSource struct {
	container *container
}

// produce streams the matching zip entry's rows into emit, treating a missing
// entry as an empty layer.
func (z *zipSource) produce(token string) RowProducer {
	return func(emit RowEmitter) error {
		err := z.container.streamCSVEntry(token, func(row []string) error { return emit(row) })
		if errors.Is(err, errEmptyContainer) {
			return nil
		}
		return err
	}
}

// Close releases the zip container's temp file.
func (z *zipSource) Close() error {
	return z.container.Close()
}

// xlsxSource streams worksheets out of a workbook that was opened once, so a
// multi-sheet routing file is iterated without re-reading the input.
type xlsxSource struct {
	container *container
	sheets    []string
}

// newXLSXSource snapshots the workbook's sheet list for token resolution.
func newXLSXSource(c *container) *xlsxSource {
	return &xlsxSource{container: c, sheets: c.sheetNames()}
}

// produce resolves token to a sheet name and streams that sheet's rows into emit,
// treating an absent sheet as an empty layer.
func (x *xlsxSource) produce(token string) RowProducer {
	sheet := x.resolveSheet(token)
	return func(emit RowEmitter) error {
		if sheet == "" {
			return nil
		}
		return x.container.streamSheet(sheet, func(row []string) error { return emit(row) })
	}
}

// resolveSheet finds the workbook sheet for token, preferring an exact (case
// insensitive) name and falling back to a substring match, so number-prefixed
// names (e.g. "1_product_master") still resolve. Returns "" when none matches.
func (x *xlsxSource) resolveSheet(token string) string {
	lower := strings.ToLower(token)
	for _, s := range x.sheets {
		if strings.ToLower(s) == lower {
			return s
		}
	}
	for _, s := range x.sheets {
		if strings.Contains(strings.ToLower(s), lower) {
			return s
		}
	}
	return ""
}

// Close releases the underlying workbook.
func (x *xlsxSource) Close() error {
	return x.container.Close()
}
