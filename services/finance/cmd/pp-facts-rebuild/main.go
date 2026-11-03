// Command pp-facts-rebuild re-aggregates the PP_PACK_* facts (DAILY/WEEKLY/MONTHLY) of an explicit
// date range from the existing staging table bi_pp_stg_pack_prod_daily, without touching Oracle
// (P007). Run it once after the WEEKLY grain lands to fill WEEKLY history; it is idempotent and
// reconciles (ETL§F) before committing.
// -d1 should be a Monday (and the first day of a month for a full MONTHLY) at or after the start of
// staging data: weeks/months are summed only from staging, so a -d1 inside a period stores it partial
// and the reconciliation (staging vs staging) cannot detect that.
// Usage: pp-facts-rebuild -d1 2025-01-01 -d2 2026-10-29
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const (
	dateLayout     = "2006-01-02"
	defaultTimeout = 30 * time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("pp-facts-rebuild: %v", err)
	}
}

// parseRange validates the -d1/-d2 flags (both required, YYYY-MM-DD, d2 not before d1).
func parseRange(d1s, d2s string) (d1, d2 time.Time, err error) {
	if d1s == "" || d2s == "" {
		return d1, d2, errors.New("-d1 and -d2 are required (YYYY-MM-DD)")
	}
	if d1, err = time.Parse(dateLayout, d1s); err != nil {
		return d1, d2, fmt.Errorf("-d1: %w", err)
	}
	if d2, err = time.Parse(dateLayout, d2s); err != nil {
		return d1, d2, fmt.Errorf("-d2: %w", err)
	}
	if d2.Before(d1) {
		return d1, d2, errors.New("-d2 before -d1")
	}
	return d1, d2, nil
}

func run() error {
	d1s := flag.String("d1", "", "first pack date (YYYY-MM-DD)")
	d2s := flag.String("d2", "", "last pack date (YYYY-MM-DD)")
	timeout := flag.Duration("timeout", defaultTimeout, "overall timeout")
	flag.Parse()
	d1, d2, err := parseRange(*d1s, *d2s)
	if err != nil {
		return err
	}
	if d1.Weekday() != time.Monday {
		log.Printf("WARNING: -d1 %s is not a Monday; the first WEEKLY period may be stored partial", *d1s)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cErr := db.Close(); cErr != nil {
			log.Printf("close db: %v", cErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var sourceID uuid.UUID
	if err = db.QueryRowContext(ctx,
		`SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE'`).Scan(&sourceID); err != nil {
		return fmt.Errorf("find HRIS_ORACLE source: %w", err)
	}
	n, err := postgres.NewBiPPStagingRepository(db).RebuildPackProdFacts(ctx, d1, d2, sourceID)
	if err != nil {
		return err
	}
	log.Printf("rebuilt %d PP_PACK fact rows for %s..%s", n, *d1s, *d2s)
	// Dashboards read mv_bi_metric_g1/g2; refresh so the rebuilt facts show without waiting for a job.
	if err = postgres.NewBiUploadRepository(db).RefreshViews(ctx); err != nil {
		return fmt.Errorf("facts committed but refresh BI views failed (safe to re-run): %w", err)
	}
	log.Printf("refreshed BI materialized views")
	return nil
}
