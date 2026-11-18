package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
)

func readPlanCSV(path string, required []string) (*mbactivesync.PlanCSV, error) {
	f, err := os.Open(path) //nolint:gosec // operator-supplied input path for a one-shot CLI
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if cErr := f.Close(); cErr != nil {
			log.Printf("WARN: close %s: %v", path, cErr)
		}
	}()
	c, err := mbactivesync.ReadPlanCSV(f, required)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func readReportPlan(rowsPath, headsPath string) (*mbactivesync.ReportPlan, error) {
	rows, err := readPlanCSV(rowsPath, mbactivesync.RowsRequiredColumns)
	if err != nil {
		return nil, err
	}
	heads, err := readPlanCSV(headsPath, mbactivesync.HeadsRequiredColumns)
	if err != nil {
		return nil, err
	}
	return mbactivesync.LoadReportPlan(rows, heads)
}

// writeReport writes report.csv (--out, else <out-dir>/sync-mb-active-report-<ts>.csv)
// and the summary next to it (.txt).
func writeReport(res *mbactivesync.ReportResult, o options) error {
	csvPath := o.out
	if csvPath == "" {
		if err := os.MkdirAll(o.outDir, 0o750); err != nil {
			return fmt.Errorf("create out dir: %w", err)
		}
		csvPath = filepath.Join(o.outDir, fmt.Sprintf("%s-report-%s.csv", cmdName, time.Now().Format("20060102-150405")))
	}
	sumPath := strings.TrimSuffix(csvPath, filepath.Ext(csvPath)) + "-summary.txt"
	if err := writeFile(csvPath, func(w io.Writer) error { return mbactivesync.WriteReportCSV(w, res.Rows) }); err != nil {
		return err
	}
	log.Printf("wrote %s", csvPath)
	if err := writeFile(sumPath, func(w io.Writer) error { return mbactivesync.WriteReportSummary(w, res) }); err != nil {
		return err
	}
	log.Printf("wrote %s", sumPath)
	return nil
}
