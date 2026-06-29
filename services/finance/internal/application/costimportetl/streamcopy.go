// Package costimportetl implements the v2 ETL bulk-import pipeline: stream a
// file from object storage into UNLOGGED staging tables via streaming readers,
// then resolve all cross-references set-based in SQL. It deliberately avoids
// loading whole files into memory (no excelize.GetRows) to keep the worker's
// resident memory bounded regardless of input size.
package costimportetl

import (
	"archive/zip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"
)

// rowSink consumes a single parsed row. Returning an error aborts the stream.
// The slice passed to a sink may be reused on the next call (excelize/csv
// ReuseRecord semantics), so a sink that retains values MUST copy them.
type rowSink func(row []string) error

// errEmptyContainer is returned when an archive contains no usable data file.
var errEmptyContainer = errors.New("container has no csv or xlsx entry")

// streamCSV reads CSV records from r and invokes sink for every data row,
// skipping the header row. It enables ReuseRecord so the underlying slice is
// reused between rows (zero per-row allocation); trailing empty fields are
// tolerated by disabling the field-count check (FieldsPerRecord = -1).
func streamCSV(r io.Reader, sink rowSink) error {
	reader := csv.NewReader(r)
	reader.ReuseRecord = true
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	headerSkipped := false
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read csv record: %w", err)
		}
		if !headerSkipped {
			headerSkipped = true
			continue
		}
		if err := sink(record); err != nil {
			return fmt.Errorf("csv row sink: %w", err)
		}
	}
}

// streamXLSXSheet iterates a sheet's rows via the excelize iterator (NEVER
// GetRows, which loads the whole sheet into memory) and feeds each data row to
// sink, skipping the header. The workbook is opened once by the caller, so all
// sheets of a routing file can be iterated in turn without re-reading the input.
func streamXLSXSheet(f *excelize.File, sheet string, sink rowSink) (err error) {
	rows, err := f.Rows(sheet)
	if err != nil {
		return fmt.Errorf("open rows for sheet %q: %w", sheet, err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()

	headerSkipped := false
	for rows.Next() {
		cols, colErr := rows.Columns()
		if colErr != nil {
			return fmt.Errorf("read columns for sheet %q: %w", sheet, colErr)
		}
		if !headerSkipped {
			headerSkipped = true
			continue
		}
		if err = sink(cols); err != nil {
			return fmt.Errorf("xlsx row sink: %w", err)
		}
	}
	if rErr := rows.Error(); rErr != nil {
		return fmt.Errorf("iterate rows for sheet %q: %w", sheet, rErr)
	}
	return err
}

// container exposes the entries of an uploaded import file in a uniform way,
// regardless of whether the source was a single .csv, a single .xlsx, or a .zip
// holding several CSV files. Close releases any temporary resources.
type container struct {
	kind     containerKind
	xlsxFile *excelize.File // set when kind == containerXLSX, opened once
	zip      *zip.Reader    // set when kind == containerZip
	tempFile *os.File       // backing temp file for the zip reader, removed on Close
}

// containerKind enumerates the supported upload container formats.
type containerKind int

const (
	// containerXLSX is a single .xlsx workbook opened once for sheet iteration.
	containerXLSX containerKind = iota
	// containerZip is a .zip archive holding one or more CSV entries.
	containerZip
)

// openContainer inspects fileName's extension and prepares a container.
//
//   - .xlsx: the workbook is opened once via excelize.OpenReader (which buffers
//     the small ~3MB routing archive) so every sheet can be iterated in turn.
//   - .zip: the stream is spooled to a temp file (archive/zip requires
//     io.ReaderAt + size); entries are still read as independent streams, so
//     peak memory stays bounded for the multi-million-row params bundle.
//
// In both cases openContainer reads rc to completion before returning, so the
// caller may close rc immediately afterwards; container.Close releases the
// workbook / temp file.
func openContainer(rc io.ReadCloser, fileName string) (*container, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".xlsx":
		f, err := excelize.OpenReader(rc)
		if err != nil {
			return nil, fmt.Errorf("open xlsx workbook: %w", err)
		}
		return &container{kind: containerXLSX, xlsxFile: f}, nil
	case ".zip":
		return openZipContainer(rc, fileName)
	default:
		return nil, fmt.Errorf("unsupported import file extension %q (want .xlsx or .zip)", ext)
	}
}

// openZipContainer spools rc to a temp file and opens a zip.Reader over it.
func openZipContainer(rc io.ReadCloser, fileName string) (*container, error) {
	tmp, err := os.CreateTemp("", "costimport-*.zip")
	if err != nil {
		return nil, fmt.Errorf("create temp file for zip: %w", err)
	}

	size, err := io.Copy(tmp, rc)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("spool zip to temp file: %w", err), removeTemp(tmp))
	}

	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open zip %q: %w", fileName, err), removeTemp(tmp))
	}

	return &container{kind: containerZip, zip: zr, tempFile: tmp}, nil
}

// removeTemp closes and deletes a temp file, returning the joined cleanup error
// (nil when both succeed). Used to release the spool file on an error path.
func removeTemp(tmp *os.File) error {
	closeErr := tmp.Close()
	rmErr := os.Remove(tmp.Name())
	return errors.Join(closeErr, rmErr)
}

// Close releases the open workbook (xlsx) or the temp file backing the zip
// reader, whichever the container holds.
func (c *container) Close() error {
	if c.xlsxFile != nil {
		if err := c.xlsxFile.Close(); err != nil {
			return fmt.Errorf("close xlsx workbook: %w", err)
		}
	}
	if c.tempFile == nil {
		return nil
	}
	name := c.tempFile.Name()
	closeErr := c.tempFile.Close()
	rmErr := os.Remove(name)
	if closeErr != nil {
		return fmt.Errorf("close temp file: %w", closeErr)
	}
	if rmErr != nil {
		return fmt.Errorf("remove temp file: %w", rmErr)
	}
	return nil
}

// streamCSVEntry streams the rows of EVERY zip entry whose name contains
// entryName (case insensitive) through sink, concatenating them in ascending
// file-name order. This lets one logical layer be split across several CSV parts
// (e.g. product_parameters_1.csv + product_parameters_2.csv) — all parts are
// merged. Each part's own header row is skipped by streamCSV. It returns
// errEmptyContainer when no entry matches (the caller treats that as 0 rows).
func (c *container) streamCSVEntry(entryName string, sink rowSink) error {
	if c.kind != containerZip {
		return fmt.Errorf("streamCSVEntry called on non-zip container")
	}
	target := strings.ToLower(entryName)

	matches := make([]*zip.File, 0, 2)
	for _, file := range c.zip.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if strings.Contains(strings.ToLower(file.Name), target) {
			matches = append(matches, file)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("zip entry matching %q: %w", entryName, errEmptyContainer)
	}
	// Deterministic order so split parts (_1, _2, …) stream in sequence.
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })

	for _, file := range matches {
		if err := streamZipFile(file, sink); err != nil {
			return err
		}
	}
	return nil
}

// streamZipFile opens one zip entry and streams its CSV rows through sink,
// always closing the entry reader.
func streamZipFile(file *zip.File, sink rowSink) error {
	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %q: %w", file.Name, err)
	}
	err = streamCSV(rc, sink)
	if cerr := rc.Close(); cerr != nil && err == nil {
		err = fmt.Errorf("close zip entry %q: %w", file.Name, cerr)
	}
	if err != nil {
		return fmt.Errorf("stream zip entry %q: %w", file.Name, err)
	}
	return nil
}

// sheetNames returns the worksheet names of an .xlsx container, or nil for a
// non-xlsx container. Used to resolve a logical layer token to a real sheet name.
func (c *container) sheetNames() []string {
	if c.xlsxFile == nil {
		return nil
	}
	return c.xlsxFile.GetSheetList()
}

// streamSheet streams the rows of the named sheet from an .xlsx container using
// the already-open workbook (no re-read), so every sheet can be iterated in turn.
func (c *container) streamSheet(sheet string, sink rowSink) error {
	if c.kind != containerXLSX {
		return fmt.Errorf("streamSheet called on non-xlsx container")
	}
	return streamXLSXSheet(c.xlsxFile, sheet, sink)
}
