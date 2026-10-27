// Command hris-att-backfill re-pulls VIEW_BI_ATT_DAILY for an explicit date range into
// bi_hr_stg_att_daily and recomputes HR_HEADCOUNT for the touched payroll periods (H033).
// The scheduled job only pulls the last 35 days (HD-04); use this once to fill earlier
// periods so ratio KPIs (OT per headcount, % overtime) have a denominator for every period
// that has OT facts. Usage: hris-att-backfill -d1 2026-07-26 -d2 2026-08-25
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const (
	dateLayout = "2006-01-02"
	// defaultTimeout bounds the whole backfill (Oracle pull + staging replace + MV refresh).
	defaultTimeout = 30 * time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("hris-att-backfill: %v", err)
	}
}

// options are the parsed command-line flags.
type options struct {
	d1s, d2s        string
	d1, d2          time.Time
	withOT, withATT bool
	timeout         time.Duration
}

func parseFlags() (options, error) {
	d1s := flag.String("d1", "", "first work date (YYYY-MM-DD)")
	d2s := flag.String("d2", "", "last work date (YYYY-MM-DD)")
	withOT := flag.Bool("ot", false, "also re-pull VIEW_BI_OT_DAILY for the same range")
	withATT := flag.Bool("att", true, "re-pull VIEW_BI_ATT_DAILY (false = OT only, e.g. legacy OT history before 2026-06-26)")
	timeout := flag.Duration("timeout", defaultTimeout, "overall timeout (long ranges on a slow Oracle need more)")
	flag.Parse()
	o := options{d1s: *d1s, d2s: *d2s, withOT: *withOT, withATT: *withATT, timeout: *timeout}
	if !o.withATT && !o.withOT {
		return o, fmt.Errorf("nothing to do: -att=false needs -ot")
	}
	var err error
	if o.d1, err = time.Parse(dateLayout, o.d1s); err != nil {
		return o, fmt.Errorf("-d1: %w", err)
	}
	if o.d2, err = time.Parse(dateLayout, o.d2s); err != nil {
		return o, fmt.Errorf("-d2: %w", err)
	}
	if o.d2.Before(o.d1) {
		return o, fmt.Errorf("-d2 before -d1")
	}
	return o, nil
}

func run() error {
	o, err := parseFlags()
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !cfg.OracleHRIS.Configured() {
		return fmt.Errorf("ORACLE_HRIS_* not configured")
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
	hris, err := oracleinfra.NewClient(cfg.OracleHRIS, zerolog.Nop())
	if err != nil {
		return fmt.Errorf("connect oracle hris: %w", err)
	}
	defer func() {
		if cErr := hris.Close(); cErr != nil {
			log.Printf("close oracle hris: %v", cErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()

	var sourceID uuid.UUID
	// Source seeded by 000582; independent of which HR jobs are active (OT-only backfill works too).
	if err = db.QueryRowContext(ctx,
		`SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE'`).Scan(&sourceID); err != nil {
		return fmt.Errorf("find HRIS source: %w", err)
	}

	if err = backfill(ctx, db, hris, o, sourceID); err != nil {
		return err
	}
	// Dashboards read mv_bi_metric_g1/g2; refresh so the backfilled facts show without waiting for a job.
	if err = postgres.NewBiUploadRepository(db).RefreshViews(ctx); err != nil {
		return fmt.Errorf("refresh BI views: %w", err)
	}
	log.Printf("refreshed BI materialized views")
	return nil
}

// backfill re-pulls ATT and/or OT for [d1, d2] into staging and recomputes their facts.
func backfill(ctx context.Context, db *postgres.DB, hris *oracleinfra.Client, o options, sourceID uuid.UUID) error {
	if o.withATT {
		rows, err := oracleinfra.NewHRISATTRepository(hris).FetchATTDaily(ctx, o.d1, o.d2)
		if err != nil {
			return fmt.Errorf("fetch ATT daily: %w", err)
		}
		n, err := postgres.NewBiHRStagingRepository(db).ReplaceATTDaily(ctx, o.d1, o.d2, rows, sourceID)
		if err != nil {
			return fmt.Errorf("replace ATT daily: %w", err)
		}
		log.Printf("backfilled %d ATT staging rows for %s..%s", n, o.d1s, o.d2s)
	}
	if !o.withOT {
		return nil
	}
	otRows, err := oracleinfra.NewHRISOTRepository(hris).FetchOTDaily(ctx, o.d1, o.d2)
	if err != nil {
		return fmt.Errorf("fetch OT daily: %w", err)
	}
	m, err := postgres.NewBiHRStagingRepository(db).ReplaceOTDaily(ctx, o.d1, o.d2, otRows, sourceID)
	if err != nil {
		return fmt.Errorf("replace OT daily: %w", err)
	}
	log.Printf("backfilled %d OT staging rows for %s..%s", m, o.d1s, o.d2s)
	return nil
}
