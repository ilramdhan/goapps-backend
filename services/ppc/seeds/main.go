// Package main provides the database seeder for the PPC service. It seeds the
// PPC-owned configuration masters (threshold config, downtime reasons, waste
// categories, machine groups). Idempotent: existing rows are skipped.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/rs/zerolog/log"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/machinesync"
	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/financeclient"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

const seedActor = "seeder"

// Deterministic markers for the idempotent end-to-end workflow demo. The demand
// contract number is the sentinel that guards against re-seeding the chain.
const (
	demoContractNo = "DEMO-WORKFLOW-001"
	demoLotNo      = "DEMO-LOT-001"
	demoMachineNo  = "DEMO-TXT01" // VARCHAR(10): exactly 10 chars.
	demoArea       = "TXT"
	activeFilter   = "active"
)

// demoMonthOf projects a deadline onto its planning month. The demand and plan
// item domains reject a month that diverges from its own deadline unless the
// carry-forward override is set, so the demo chain must derive each month from
// the deadline it was built with rather than pin a literal that goes stale as
// wall-clock time advances.
func demoMonthOf(deadline time.Time) string { return deadline.Format("2006-01") }

type thresholdSeed struct {
	level   string
	unit    string
	warning float64
	block   float64
	notes   string
}

type downtimeReasonSeed struct {
	area           string
	code           string
	name           string
	category       string
	excludeFromEff bool
	sortOrder      int
}

type wasteCategorySeed struct {
	area      string
	wasteType string
	code      string
	name      string
	sortOrder int
}

type machineGroupSeed struct {
	name string
	area string
}

type lookupSeed struct {
	category  string
	code      string
	label     string
	sortOrder int
}

type shiftSeed struct {
	code      string
	name      string
	startTime string
	endTime   string
}

// productConfigSeed is one product_ppc_config row. The product is a SOFT
// reference into finance cost_product_master (no FK — separate database), so the
// sys id is validated against finance at seed time and the row is skipped when
// the product is gone or inactive. The machine group is resolved by
// (name, area) rather than by id: ids are BIGSERIAL and differ per environment.
type productConfigSeed struct {
	productSysID int64
	// productName mirrors finance cpm_product_name at the time this seed was
	// written. It is only a fallback for denier derivation and log output — the
	// live finance name wins whenever finance is reachable.
	productName    string
	groupName      string
	groupArea      string
	commodityWatch bool
	yieldStd       float64
	bufferRmPct    float64
	axYieldPct     float64
}

// System-level over-production threshold: warn at 3%, block at 6% (PCT).
var thresholdSeeds = []thresholdSeed{
	{"SYSTEM", "PCT", 3, 6, "Default system-level over-production threshold"},
}

// TXT downtime reasons. POWER_FAILURE is excluded from efficiency calc.
var downtimeReasonSeeds = []downtimeReasonSeed{
	{"TXT", "XST", "Extruder Stop", "MACHINE_DOWN", false, 1},
	{"TXT", "LB", "Lap / Break", "PRODUCTION_LOSS", false, 2},
	{"TXT", "TP", "Tension Problem", "PRODUCTION_LOSS", false, 3},
	{"TXT", "FUSE", "Fuse Blown", "MACHINE_DOWN", false, 4},
	{"TXT", "BOWL", "Bowl Change", "IDLE_POSITION", false, 5},
	{"TXT", "POWER_FAILURE", "Power Failure", "MACHINE_DOWN", true, 6},
	// SPG production-loss reasons (6 categories, PRD 13). Power Plants + Electric
	// (utility outages) are excluded from efficiency like TXT Power Failure.
	{"SPG", "REGULER", "Reguler", "PRODUCTION_LOSS", false, 1},
	{"SPG", "CPF", "CPF", "PRODUCTION_LOSS", false, 2},
	{"SPG", "UTILITY", "Utility", "MACHINE_DOWN", false, 3},
	{"SPG", "ELECTRIC", "Electric", "MACHINE_DOWN", true, 4},
	{"SPG", "POWER_PLANTS", "Power Plants", "MACHINE_DOWN", true, 5},
	{"SPG", "OTHERS", "Others", "PRODUCTION_LOSS", false, 6},
	// TWT downtime reasons (Phase 3, PRD 13). Power/Electric utility outages are
	// excluded from efficiency like TXT/SPG.
	{"TWT", "MC_DOWN", "Machine Down", "MACHINE_DOWN", false, 1},
	{"TWT", "PROD_LOSS", "Production Loss", "PRODUCTION_LOSS", false, 2},
	{"TWT", "PROD_MIX", "Production Mix Change", "PRODUCTION_LOSS", false, 3},
	{"TWT", "MAINTENANCE", "Maintenance", "MACHINE_DOWN", false, 4},
	{"TWT", "ELECTRIC", "Electric", "MACHINE_DOWN", true, 5},
	{"TWT", "POWER_FAILURE", "Power Failure", "MACHINE_DOWN", true, 6},
}

// TXT + SPG waste categories. SPG has 8 WASTE categories (PRD 13) plus DOWNGRADE
// reasons recorded in the same master (type DOWNGRADE).
var wasteCategorySeeds = []wasteCategorySeed{
	{"TXT", "WASTE", "DTY", "DTY Waste", 1},
	{"TXT", "WASTE", "POY", "POY Waste", 2},
	{"TXT", "WASTE", "TAKE_UP", "Take-up Waste", 3},
	{"TXT", "WASTE", "FLYING_FILAMENT", "Flying Filament Waste", 4},
	// SPG 8-category waste (each also tracked with/without upsets at report time).
	{"SPG", "WASTE", "SPINNING", "Spinning Waste", 1},
	{"SPG", "WASTE", "TAKE_UP", "Take Up Waste", 2},
	{"SPG", "WASTE", "STRIPPING", "Stripping Waste", 3},
	{"SPG", "WASTE", "PAPER_TUBE_CLEAN", "Paper Tube Cleaning Waste", 4},
	{"SPG", "WASTE", "UPSETS", "Upsets Waste", 5},
	{"SPG", "WASTE", "PROD_MIX_CHANGE", "Production Mix Change Waste", 6},
	{"SPG", "WASTE", "LABORATORY", "Laboratory Waste", 7},
	{"SPG", "WASTE", "SOLID", "Solid Waste", 8},
	// SPG downgrade reasons (type DOWNGRADE) — B/C-grade recap (PRD 13 Downgrade).
	{"SPG", "DOWNGRADE", "RM", "Downgrade - Rm", 1},
	{"SPG", "DOWNGRADE", "CC", "Downgrade - CC", 2},
	{"SPG", "DOWNGRADE", "PC", "Downgrade - PC", 3},
	{"SPG", "DOWNGRADE", "BB", "Downgrade - BB", 4},
	{"SPG", "DOWNGRADE", "MB_FAIL", "Downgrade - MB Fail", 5},
	// TWT waste categories (Phase 3, PRD 13) — per twisting machine-type stage.
	{"TWT", "WASTE", "WINDING", "Winding Waste", 1},
	{"TWT", "WASTE", "TWISTING", "Twisting Waste", 2},
	{"TWT", "WASTE", "ASSEMBLY", "Assembly Waste", 3},
	{"TWT", "WASTE", "SETTING", "Heat Setting Waste", 4},
	{"TWT", "WASTE", "PACKING", "Packing Waste", 5},
	// TWT downgrade reasons (type DOWNGRADE).
	{"TWT", "DOWNGRADE", "QUALITY", "Downgrade - Quality", 1},
	{"TWT", "DOWNGRADE", "TENSION", "Downgrade - Tension", 2},
}

// TXT + SPG machine groups. SPG groups model spinning lines.
var machineGroupSeeds = []machineGroupSeed{
	{"TXT Line A", "TXT"},
	{"TXT Line B", "TXT"},
	{"SPG Line 1", "SPG"},
	{"SPG Line 2", "SPG"},
	// TWT machine-type groups (Phase 3, PRD 07/13). Each twisting machine type is
	// its own group for downtime/waste rollup.
	{"Cops Winder", "TWT"},
	{"PT/ST", "TWT"},
	{"Assembly Winder", "TWT"},
	{"Carpet Twister", "TWT"},
	{"Bluemoon", "TWT"},
	{"Meerabah", "TWT"},
	{"SSM", "TWT"},
	{"ACY", "TWT"},
	{"Ply", "TWT"},
}

// PPC lookup master seeds (DISPLAY metadata only). Codes MUST equal the Go enum
// string constants the frontend sends back; the backend still validates against
// those enum constants — this table never drives a business decision. Labels are
// human-friendly for dropdowns.
var lookupSeeds = []lookupSeed{
	// PPC_AREA — production areas.
	{"PPC_AREA", "TXT", "Texturizing (TXT)", 1},
	{"PPC_AREA", "SPG", "Spinning (SPG)", 2},
	{"PPC_AREA", "TWT", "Twisting (TWT)", 3},
	// PPC_DEMAND_TYPE — demand origin.
	{"PPC_DEMAND_TYPE", "CONTRACT", "Contract", 1},
	{"PPC_DEMAND_TYPE", "MTS", "Make to Stock", 2},
	{"PPC_DEMAND_TYPE", "SAMPLE", "Sample", 3},
	// PPC_DEMAND_SUBTYPE — demand sub-classification.
	{"PPC_DEMAND_SUBTYPE", "CF_EXPORT", "Carry-Forward Export", 1},
	{"PPC_DEMAND_SUBTYPE", "NEW_EXPORT", "New Export", 2},
	{"PPC_DEMAND_SUBTYPE", "LOCAL", "Local", 3},
	{"PPC_DEMAND_SUBTYPE", "INTERNAL", "Internal", 4},
	// PPC_GRADE_REQ — grade requirement clause.
	{"PPC_GRADE_REQ", "AX_ONLY", "AX Only", 1},
	{"PPC_GRADE_REQ", "AX_AM_CLAUSE", "AX + AM Clause", 2},
	{"PPC_GRADE_REQ", "NONE", "None", 3},
	// PPC_PLANITEM_TYPE — plan-item cascade type.
	{"PPC_PLANITEM_TYPE", "FG_DELIVERY", "FG Delivery", 1},
	{"PPC_PLANITEM_TYPE", "INTERMEDIATE", "Intermediate", 2},
	{"PPC_PLANITEM_TYPE", "MTS", "Make to Stock", 3},
	// PPC_RM_SOURCE — raw-material sourcing.
	{"PPC_RM_SOURCE", "STORE", "Store", 1},
	{"PPC_RM_SOURCE", "CAPTIVE", "Captive", 2},
	{"PPC_RM_SOURCE", "MIXED", "Mixed", 3},
	// PPC_PROD_CATEGORY — production category (efficiency branch).
	{"PPC_PROD_CATEGORY", "NORMAL", "Normal", 1},
	{"PPC_PROD_CATEGORY", "B_TO_B", "Back to Back", 2},
	{"PPC_PROD_CATEGORY", "APQ", "APQ", 3},
	{"PPC_PROD_CATEGORY", "TRIAL", "Trial", 4},
	{"PPC_PROD_CATEGORY", "SMALL_LOT", "Small Lot", 5},
	// PPC_QTY_SOURCE — production-actual quantity source (two-axis).
	{"PPC_QTY_SOURCE", "BOBBIN", "Bobbin (ETL)", 1},
	{"PPC_QTY_SOURCE", "ADJUSTED", "Adjusted (Manual)", 2},
	// PPC_WO_REF_TYPE — work-order reference type.
	{"PPC_WO_REF_TYPE", "TEMPLATE", "Template", 1},
	{"PPC_WO_REF_TYPE", "CONTINUATION", "Continuation", 2},
	// PPC_THRESHOLD_UNIT — overrun threshold unit.
	{"PPC_THRESHOLD_UNIT", "PCT", "Percent (%)", 1},
	{"PPC_THRESHOLD_UNIT", "DOFF", "Doff", 2},
}

// PPC shift master seeds (PRD 09-master-data shift config). Shift 3 crosses
// midnight (22:00 -> 06:00 next day).
var shiftSeeds = []shiftSeed{
	{"1", "Shift 1", "06:00", "14:00"},
	{"2", "Shift 2", "14:00", "22:00"},
	{"3", "Shift 3", "22:00", "06:00"},
}

// product_ppc_config seeds.
//
// Product sys ids are REAL finance cost_product_master rows (verified active in
// finance_db), not invented numbers — a made-up sys id would look valid in
// ppc_db and only fail much later at the product-picker join. The first three
// mirror the products/machine groups already used by production_plan_item, so
// the seeded config agrees with existing planning data instead of contradicting
// it; the rest spread the config across the remaining SPG/TXT/TWT groups so
// every area has at least one configured product.
//
// Value provenance — read this before trusting a number:
//   - ppc_ax_yield_pct: the ONLY PRD-sourced figure. 03-layer1-demand.md pins
//     historical %AX per product at 0.75–0.84 and uses it as
//     est_prod_needed = pd_qty_remaining / ppc_ax_yield_pct. Every value below
//     sits inside that band; the split across it (finer denier / dyed shade =
//     lower yield) is a plausible ordering, not measured data.
//   - ppc_yield_std: NO PRD figure exists. Assumed process yield, set just above
//     the product's %AX because standard yield counts sellable output (AX + the
//     released off-grades) while %AX counts prime only.
//   - ppc_buffer_rm_pct: NO PRD figure exists. Assumed RM over-issue buffer,
//     2–4% — larger for dyed/specialty items that scrap more at start-up.
//   - ppc_denier: NOT hardcoded. Derived at seed time from the first numeric
//     token of the live finance product name (POY 300/96/... -> 300), matching
//     the industry convention that the leading token is the denier. Falls back
//     to the name recorded on the seed when finance is unreachable.
//   - ppc_price_sell: intentionally left NULL. A wrong sell price is worse than
//     an absent one — it silently feeds commodity-watch reporting.
//
// PPC corrects all of these on the product-config screen; they exist so the
// planning screens have non-empty, defensible data on a fresh database.
var productConfigSeeds = []productConfigSeed{
	// Already referenced by production_plan_item (product -> machine group).
	{101348, "TTY 480/216-120/TBL/DBR/NI/ACDH/SN/1/S", "SSM", "TWT", true, 0.86, 0.04, 0.80},
	{97503, "PTY 480/216/TBL/DBR/LIM/GTDH/N/1/S", "ACY", "TWT", true, 0.84, 0.04, 0.78},
	{91792, "POY 380/108/TBL/DBR/SIM/NS/2/O", "TXT Line B", "TXT", false, 0.88, 0.03, 0.82},
	// Spread across the remaining areas/groups.
	{90740, "POY 150/72/RND/SD/SIM/NS/1/O", "TXT Line A", "TXT", false, 0.89, 0.02, 0.84},
	{92438, "POY 300/96/RND/RSD-FR/SIM/NS/1/O", "SPG Line 1", "SPG", false, 0.87, 0.03, 0.81},
	{92045, "POY 300/192/RND/SD/SIM/NS/1/O", "SPG Line 2", "SPG", false, 0.85, 0.03, 0.79},
	{92882, "PTY 150/48/RND/SD/HIM/DH/N/1/Z-for compare", "Ply", "TWT", false, 0.86, 0.03, 0.80},
	{88231, "ACY 150/48+40/RND/SD-PU/HIM/DH/N/1/ZO", "Bluemoon", "TWT", true, 0.83, 0.04, 0.76},
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	db, err := sql.Open("pgx", cfg.Database.ConnectionString())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("Failed to close database connection")
		}
	}()

	// Machine sync + finance master reads + the demand→plan→WO chain take longer
	// than the master seeders, so allow a wider window.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to ping database")
		return
	}

	seedThresholds(ctx, db)
	seedDowntimeReasons(ctx, db)
	seedWasteCategories(ctx, db)
	seedMachineGroups(ctx, db)
	seedProductPPCConfig(ctx, db, cfg)
	seedLookups(ctx, db)
	seedShifts(ctx, db)

	// The machine sync is master/reference data, not a demo fixture: it upserts the
	// real TXT/SPG/TWT machines from finance + Oracle. It therefore sits OUTSIDE the
	// demo gate below and runs in every environment. It also runs BEFORE the demo
	// block because the demo chain resolves a machine and a machine group from the
	// rows this sync populates.
	syncMachinesForSeed(ctx, db, cfg)

	// The workflow demo (and the demo lot it seeds) are development-only fixtures:
	// they inject real business records — a confirmed demand, a plan item, a work
	// order and a lot_master row — so they must never run against a staging or
	// production database. Gated with the same condition IAM's seeder uses. The
	// seed Job supplies APP_ENV from the namespace via fieldRef, so an empty value
	// means a local run and is treated as development. The master/reference
	// seeders above stay ungated: they must run in every environment.
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "development" || appEnv == "" {
		seedWorkflowDemo(ctx, db, cfg)
	} else {
		log.Info().Str("app_env", appEnv).
			Msg("Skipping workflow demo seed: demo fixtures run in development only")
	}
}

func seedLookups(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding PPC lookup master")
	inserted, skipped := 0, 0
	for _, s := range lookupSeeds {
		res, err := db.ExecContext(ctx,
			`INSERT INTO ppc_lookup
			   (pl_category, pl_code, pl_label, pl_sort_order, pl_created_by)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (pl_category, pl_code) DO NOTHING`,
			s.category, s.code, s.label, s.sortOrder, seedActor,
		)
		inserted, skipped = tally(res, err, inserted, skipped, "lookup "+s.category+"/"+s.code)
	}
	report("ppc_lookup", inserted, skipped, len(lookupSeeds))
}

func seedShifts(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding PPC shift master")
	inserted, skipped := 0, 0
	for _, s := range shiftSeeds {
		res, err := db.ExecContext(ctx,
			`INSERT INTO ppc_shift
			   (ps_code, ps_name, ps_start_time, ps_end_time, ps_created_by)
			 VALUES ($1, $2, $3::time, $4::time, $5)
			 ON CONFLICT (ps_code) DO NOTHING`,
			s.code, s.name, s.startTime, s.endTime, seedActor,
		)
		inserted, skipped = tally(res, err, inserted, skipped, "shift "+s.code)
	}
	report("ppc_shift", inserted, skipped, len(shiftSeeds))
}

// demoChainInput carries the resolved references needed to build the end-to-end
// workflow chain (demand → plan item → work order).
type demoChainInput struct {
	productSysID int64
	machineID    int64
	groupID      int64
	lotNo        string
	crhHeadID    int64
	crhVersion   int32
}

// seedWorkflowDemo seeds ONE realistic end-to-end workflow chain so a user can
// click through the full flow. It expects the machine sync to have already run
// (main calls syncMachinesForSeed first) because the chain resolves a machine and
// a machine group from the machine table. It is idempotent: the demand sentinel
// contract number guards re-runs, and every supporting row uses ON CONFLICT /
// existence checks. Finance being unreachable degrades gracefully — the chain is
// skipped rather than crashing the seeder.
func seedWorkflowDemo(ctx context.Context, db *sql.DB, cfg *config.Config) {
	log.Info().Msg("Seeding end-to-end workflow demo")

	if workflowDemoExists(ctx, db) {
		fmt.Printf("\nworkflow demo already seeded (contract %s), skipping chain\n", demoContractNo)
		return
	}

	lookupClient, err := financeclient.New(
		cfg.FinanceClient.Host, cfg.FinanceClient.Port,
		cfg.FinanceClient.InternalServiceToken, cfg.Server.GRPCTimeout,
	)
	if err != nil {
		log.Warn().Err(err).Msg("workflow demo: finance lookup client init failed, skipping chain")
		return
	}
	defer closeSeedClient("finance lookup client", lookupClient.Close)

	productSysID, ok := fetchDemoProduct(ctx, lookupClient)
	if !ok {
		log.Warn().Msg("workflow demo: no finance product available (degraded/unreachable), skipping chain")
		return
	}

	machineID, ok := resolveDemoMachine(ctx, db)
	if !ok {
		log.Warn().Msg("workflow demo: no TXT machine available, skipping chain")
		return
	}
	groupID, ok := demoMachineGroupID(ctx, db)
	if !ok {
		log.Warn().Msg("workflow demo: no TXT machine group available, skipping chain")
		return
	}
	seedDemoLot(ctx, db)

	headID, version := demoRoute(ctx, lookupClient, productSysID)
	if err := createDemoWorkflowChain(ctx, postgres.NewDBFromSQL(db), demoChainInput{
		productSysID: productSysID,
		machineID:    machineID,
		groupID:      groupID,
		lotNo:        demoLotNo,
		crhHeadID:    headID,
		crhVersion:   version,
	}); err != nil {
		log.Error().Err(err).Msg("workflow demo: failed to build chain")
		return
	}
	fmt.Printf("\nworkflow demo seeded! demand(%s) → confirmed → plan item(+intermediate) → work order(approved)\n", demoContractNo)
}

// syncMachinesForSeed runs the finance + Oracle machine sync so the machine
// table is populated with real TXT/SPG/TWT machines. The PPC machine_area is
// derived only from Oracle TXTMACH, so a finance-only sync inserts nothing —
// Oracle is required for real machines. Both sources degrade independently: a
// nil/unreachable Oracle falls back to finance-only, and an unreachable finance
// degrades to a no-op sync. Neither failure aborts the seeder.
func syncMachinesForSeed(ctx context.Context, db *sql.DB, cfg *config.Config) {
	log.Info().Msg("Syncing machines from finance + Oracle master")
	machineClient, err := financeclient.NewMachineClient(
		cfg.FinanceClient.Host, cfg.FinanceClient.Port,
		cfg.FinanceClient.InternalServiceToken, cfg.Server.GRPCTimeout,
	)
	if err != nil {
		log.Warn().Err(err).Msg("machine sync: client init failed, skipping sync")
		return
	}
	defer closeSeedClient("finance machine client", machineClient.Close)

	oracleSrc := seedOracleSource(cfg)

	pdb := postgres.NewDBFromSQL(db)
	usecase := machinesync.NewUsecase(postgres.NewMachineRepository(pdb), machineClient, oracleSrc)
	res, err := usecase.Sync(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("machine sync: failed, continuing degraded")
		return
	}
	log.Info().
		Int("inserted", res.Inserted).Int("updated", res.Updated).Int("skipped", res.Skipped).
		Bool("finance_used", res.FinanceUsed).Bool("oracle_used", res.OracleUsed).
		Msg("machine sync complete")
	fmt.Printf("\nmachine sync completed! inserted=%d updated=%d skipped=%d finance=%t oracle=%t\n",
		res.Inserted, res.Updated, res.Skipped, res.FinanceUsed, res.OracleUsed)
}

// seedOracleSource builds an Oracle machine source for the seed sync, or nil
// when Oracle is unconfigured/unreachable (the sync then runs finance-only).
// The client is intentionally not closed here — it lives for the single sync
// call in this short-lived seeder process and the OS reclaims it on exit.
func seedOracleSource(cfg *config.Config) machinesync.OracleMachineSource {
	client, err := oracle.New(cfg.Oracle)
	if err != nil {
		log.Warn().Err(err).Msg("machine sync: Oracle unavailable, running finance-only")
		return nil
	}
	if client == nil {
		return nil
	}
	return client
}

// workflowDemoExists reports whether the sentinel demand already exists.
func workflowDemoExists(ctx context.Context, db *sql.DB) bool {
	var count int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM production_demand WHERE pd_contract_no = $1`, demoContractNo,
	).Scan(&count)
	if err != nil {
		log.Error().Err(err).Msg("workflow demo: existence check failed")
		return true // fail safe: do not attempt to seed on an unknown state.
	}
	return count > 0
}

// fetchDemoProduct returns one real, active finance product sys id, or ok=false
// when finance is degraded/unreachable or has no active product.
func fetchDemoProduct(ctx context.Context, client *financeclient.Client) (int64, bool) {
	resp, err := client.ListProducts(ctx, &financev1.ListCostProductMasterForPPCRequest{
		Page:         1,
		PageSize:     1,
		ActiveFilter: activeFilter,
	})
	if err != nil {
		log.Warn().Err(err).Msg("workflow demo: finance product lookup failed")
		return 0, false
	}
	data := resp.GetData()
	if len(data) == 0 || data[0].GetProductSysId() <= 0 {
		return 0, false
	}
	sysID := data[0].GetProductSysId()
	log.Info().Int64("cpm_product_sys_id", sysID).Str("product_code", data[0].GetProductCode()).
		Msg("workflow demo: using finance product")
	return sysID, true
}

// resolveDemoMachine returns a usable TXT machine id. It prefers a real synced
// machine; when none exists (e.g. Oracle-less sync yields no area) it inserts a
// deterministic demo TXT machine (idempotent) so the chain can still complete.
func resolveDemoMachine(ctx context.Context, db *sql.DB) (int64, bool) {
	id, ok := selectTxtMachine(ctx, db)
	if ok {
		return id, true
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO machine (machine_no, machine_area, machine_is_active, created_by)
		 VALUES ($1, $2, TRUE, $3)
		 ON CONFLICT (machine_no) DO NOTHING`,
		demoMachineNo, demoArea, seedActor,
	); err != nil {
		log.Error().Err(err).Msg("workflow demo: failed to seed demo machine")
		return 0, false
	}
	return selectTxtMachine(ctx, db)
}

// selectTxtMachine returns the first active TXT machine id, or ok=false if none.
func selectTxtMachine(ctx context.Context, db *sql.DB) (int64, bool) {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT machine_id FROM machine WHERE machine_area = $1 AND machine_is_active ORDER BY machine_id LIMIT 1`,
		demoArea,
	).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false
	case err != nil:
		log.Error().Err(err).Msg("workflow demo: TXT machine lookup failed")
		return 0, false
	default:
		return id, true
	}
}

// demoMachineGroupID returns an existing TXT machine-group id (seeded by
// seedMachineGroups), or ok=false when none exists.
func demoMachineGroupID(ctx context.Context, db *sql.DB) (int64, bool) {
	var id int64
	err := db.QueryRowContext(ctx,
		`SELECT group_id FROM machine_group WHERE group_area = $1 ORDER BY group_id LIMIT 1`, demoArea,
	).Scan(&id)
	if err != nil {
		log.Error().Err(err).Msg("workflow demo: TXT machine group lookup failed")
		return 0, false
	}
	return id, true
}

// seedDemoLot inserts the deterministic demo lot (idempotent).
func seedDemoLot(ctx context.Context, db *sql.DB) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO lot_master
		   (lm_lot_no, lm_item_code, lm_shade_code, lm_std_weight_full, lm_std_weight_unfull, lm_created_by)
		 VALUES ($1, 'DEMO-ITEM', 'DEMO-SHADE', 10.0, 8.0, $2)
		 ON CONFLICT (lm_lot_no) DO NOTHING`,
		demoLotNo, seedActor,
	)
	inserted, skipped := tally(res, err, 0, 0, "lot "+demoLotNo)
	report("lot_master (demo)", inserted, skipped, 1)
}

// demoRoute returns a real released-route head id + version for the product, or
// falls back to (1, 1) when finance has no route or is degraded (the WO create
// only requires positive values; the route snapshot is best-effort).
func demoRoute(ctx context.Context, client *financeclient.Client, productSysID int64) (int64, int32) {
	resp, err := client.GetProductRoute(ctx, productSysID)
	if err != nil {
		log.Warn().Err(err).Int64("cpm_product_sys_id", productSysID).
			Msg("workflow demo: route lookup failed, using fallback head/version 1/1")
		return 1, 1
	}
	route := resp.GetData()
	if route == nil || route.GetHeadId() <= 0 || route.GetVersion() <= 0 {
		return 1, 1
	}
	return route.GetHeadId(), route.GetVersion()
}

// createDemoWorkflowChain builds the demand → confirm → plan item (with cascaded
// intermediate) → work order → submit → PC approve → PM approve chain using the
// application services, mirroring the planning-flow integration test.
func createDemoWorkflowChain(ctx context.Context, pdb *postgres.DB, in demoChainInput) error {
	demandSvc := demandapp.NewService(postgres.NewDemandRepository(pdb), nil, nil)
	demandDeadline := time.Now().Add(30 * 24 * time.Hour)
	dmd, err := demandSvc.Create(ctx, demandapp.CreateCommand{
		Type:            demanddomain.TypeContract,
		SubType:         demanddomain.SubTypeNewExport,
		Source:          demanddomain.SourceManual,
		CpmProductSysID: in.productSysID,
		QtyOriginal:     1000,
		Deadline:        demandDeadline,
		GradeReq:        demanddomain.GradeReqNone,
		ContractNo:      demoContractNo,
		Month:           demoMonthOf(demandDeadline),
		CreatedBy:       1,
	})
	if err != nil {
		return fmt.Errorf("create demand: %w", err)
	}
	if _, err = demandSvc.Confirm(ctx, dmd.ID(), 1); err != nil {
		return fmt.Errorf("confirm demand: %w", err)
	}

	demandID := dmd.ID()
	planSvc := planitemapp.NewService(postgres.NewPlanItemRepository(pdb), nil, nil)
	planDeadline := time.Now().Add(25 * 24 * time.Hour)
	planRes, err := planSvc.Create(ctx, planitemapp.CreateCommand{
		CpmProductSysID: in.productSysID,
		Type:            planitemdomain.TypeFGDelivery,
		DemandID:        &demandID,
		QtyTarget:       1000,
		Deadline:        planDeadline,
		RMSource:        planitemdomain.RMSourceStore,
		MachineGroupID:  in.groupID,
		Month:           demoMonthOf(planDeadline),
		CreatedBy:       1,
	})
	if err != nil {
		return fmt.Errorf("create plan item: %w", err)
	}

	return createDemoWorkOrder(ctx, pdb, in, planRes.Item.ID())
}

// createDemoWorkOrder creates the WO for the plan item and drives it through the
// sequential PC → PM approval to APPROVED.
func createDemoWorkOrder(ctx context.Context, pdb *postgres.DB, in demoChainInput, planItemID int64) error {
	woSvc := workorderapp.NewService(postgres.NewWorkOrderRepository(pdb), workorderapp.Deps{
		Machines:  postgres.NewMachineAreaLookup(postgres.NewMachineRepository(pdb)),
		Lots:      postgres.NewLotExistsLookup(postgres.NewLotRepository(pdb)),
		PlanItems: postgres.NewPlanItemProductLookup(pdb),
	})
	wo, err := woSvc.Create(ctx, workorderapp.CreateCommand{
		AreaCode:   demoArea,
		PlanItemID: planItemID,
		MachineID:  in.machineID,
		CrhHeadID:  in.crhHeadID,
		CrhVersion: in.crhVersion,
		LotNo:      in.lotNo,
		QtyTarget:  1000,
		Deadline:   time.Now().Add(20 * 24 * time.Hour),
		CreatedBy:  1,
	})
	if err != nil {
		return fmt.Errorf("create work order: %w", err)
	}
	if _, err = woSvc.Submit(ctx, wo.ID(), nil); err != nil {
		return fmt.Errorf("submit work order: %w", err)
	}
	if _, err = woSvc.ApproveWO(ctx, wo.ID(), workorderdomain.ApprovalSidePC, 2); err != nil {
		return fmt.Errorf("PC approve work order: %w", err)
	}
	if _, err = woSvc.ApproveWO(ctx, wo.ID(), workorderdomain.ApprovalSidePM, 3); err != nil {
		return fmt.Errorf("PM approve work order: %w", err)
	}
	return nil
}

// closeSeedClient runs a client Close and logs any error without failing.
func closeSeedClient(name string, closeFn func() error) {
	if err := closeFn(); err != nil {
		log.Warn().Err(err).Msgf("workflow demo: failed to close %s", name)
	}
}

func seedThresholds(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding overrun threshold config")
	inserted, skipped := 0, 0
	for _, s := range thresholdSeeds {
		res, err := db.ExecContext(ctx,
			`INSERT INTO overrun_threshold_config
			   (otc_level, otc_ref_id, otc_threshold_unit, otc_warning_value, otc_block_value, otc_notes, otc_created_by)
			 SELECT $1::varchar, NULL::bigint, $2::varchar, $3::numeric, $4::numeric, $5::text, $6::varchar
			 WHERE NOT EXISTS (
			   SELECT 1 FROM overrun_threshold_config WHERE otc_level = $1::varchar AND otc_ref_id IS NULL
			 )`,
			s.level, s.unit, s.warning, s.block, s.notes, seedActor,
		)
		inserted, skipped = tally(res, err, inserted, skipped, "threshold "+s.level)
	}
	report("overrun_threshold_config", inserted, skipped, len(thresholdSeeds))
}

func seedDowntimeReasons(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding downtime reason master")
	inserted, skipped := 0, 0
	for _, s := range downtimeReasonSeeds {
		res, err := db.ExecContext(ctx,
			`INSERT INTO downtime_reason_master
			   (drm_area, drm_code, drm_name, drm_category, drm_is_exclude_from_eff, drm_sort_order, drm_created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (drm_area, drm_code) DO NOTHING`,
			s.area, s.code, s.name, s.category, s.excludeFromEff, s.sortOrder, seedActor,
		)
		inserted, skipped = tally(res, err, inserted, skipped, "downtime "+s.code)
	}
	report("downtime_reason_master", inserted, skipped, len(downtimeReasonSeeds))
}

func seedWasteCategories(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding waste category master")
	inserted, skipped := 0, 0
	for _, s := range wasteCategorySeeds {
		res, err := db.ExecContext(ctx,
			`INSERT INTO waste_category_master
			   (wcm_area, wcm_type, wcm_code, wcm_name, wcm_sort_order, wcm_created_by)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (wcm_area, wcm_type, wcm_code) DO NOTHING`,
			s.area, s.wasteType, s.code, s.name, s.sortOrder, seedActor,
		)
		inserted, skipped = tally(res, err, inserted, skipped, "waste "+s.code)
	}
	report("waste_category_master", inserted, skipped, len(wasteCategorySeeds))
}

func seedMachineGroups(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding machine groups")
	inserted, skipped := 0, 0
	for _, s := range machineGroupSeeds {
		res, err := db.ExecContext(ctx,
			`INSERT INTO machine_group (group_name, group_area, created_by)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (group_name, group_area) DO NOTHING`,
			s.name, s.area, seedActor,
		)
		inserted, skipped = tally(res, err, inserted, skipped, "machine group "+s.name)
	}
	report("machine_group", inserted, skipped, len(machineGroupSeeds))
}

// seedProductPPCConfig seeds the PPC-side product config extension.
//
// Two soft references must hold before a row is written: the finance product
// must exist and be active (validated over gRPC), and the machine group must
// exist in this database. Either missing skips just that row — a fresh database
// without finance running still gets every other master seeded. Idempotent via
// ON CONFLICT on the unique ppc_cpm_product_sys_id.
func seedProductPPCConfig(ctx context.Context, db *sql.DB, cfg *config.Config) {
	log.Info().Msg("Seeding product PPC config")

	client, err := financeclient.New(
		cfg.FinanceClient.Host, cfg.FinanceClient.Port,
		cfg.FinanceClient.InternalServiceToken, cfg.Server.GRPCTimeout,
	)
	if err != nil {
		log.Warn().Err(err).Msg("product config: finance client init failed, skipping")
		return
	}
	defer closeSeedClient("finance product config client", client.Close)

	products := financeProductNames(ctx, client)
	if products == nil {
		log.Warn().Msg("product config: finance unreachable/degraded, skipping (products cannot be validated)")
		return
	}

	inserted, skipped := 0, 0
	for _, s := range productConfigSeeds {
		name, ok := products[s.productSysID]
		if !ok {
			log.Warn().Int64("cpm_product_sys_id", s.productSysID).
				Msg("product config: finance product missing/inactive, skipping row")
			skipped++
			continue
		}
		if name == "" {
			name = s.productName
		}
		inserted, skipped = insertProductConfig(ctx, db, s, name, inserted, skipped)
	}
	report("product_ppc_config", inserted, skipped, len(productConfigSeeds))
}

// insertProductConfig writes one config row, resolving the machine group by
// (name, area) inside the statement so a missing group inserts nothing instead
// of writing a dangling reference.
func insertProductConfig(
	ctx context.Context, db *sql.DB, s productConfigSeed, productName string, inserted, skipped int,
) (int, int) {
	label := fmt.Sprintf("product config %d", s.productSysID)
	denier := deriveDenier(productName)

	res, err := db.ExecContext(ctx,
		`INSERT INTO product_ppc_config
		   (ppc_cpm_product_sys_id, ppc_is_commodity_watch, ppc_machine_group_id,
		    ppc_yield_std, ppc_buffer_rm_pct, ppc_ax_yield_pct, ppc_denier, ppc_created_by)
		 SELECT $1::bigint, $2::boolean, g.group_id,
		        $5::numeric, $6::numeric, $7::numeric, $8::numeric, $9::varchar
		 FROM machine_group g
		 WHERE g.group_name = $3::varchar AND g.group_area = $4::varchar
		 ON CONFLICT (ppc_cpm_product_sys_id) DO NOTHING`,
		s.productSysID, s.commodityWatch, s.groupName, s.groupArea,
		s.yieldStd, s.bufferRmPct, s.axYieldPct, denier, seedActor,
	)
	if err == nil {
		if affected, rowsErr := res.RowsAffected(); rowsErr == nil && affected == 0 {
			// Distinguish "already seeded" from "machine group absent": only the
			// latter is a real gap worth surfacing.
			if !machineGroupExists(ctx, db, s.groupName, s.groupArea) {
				log.Warn().Str("group", s.groupName).Str("area", s.groupArea).
					Msg("product config: machine group missing, skipping row")
			}
		}
	}
	return tally(res, err, inserted, skipped, label)
}

// machineGroupExists reports whether a machine group exists for (name, area).
// A lookup failure reports true so the caller stays quiet rather than blaming a
// missing group for an unrelated error.
func machineGroupExists(ctx context.Context, db *sql.DB, name, area string) bool {
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM machine_group WHERE group_name = $1 AND group_area = $2`, name, area,
	).Scan(&count); err != nil {
		log.Error().Err(err).Msg("product config: machine group lookup failed")
		return true
	}
	return count > 0
}

// financeProductNames resolves the seeded product sys ids against finance,
// returning sys id -> product name for the ones that exist and are active.
// A nil map means finance itself could not be reached — the caller must skip
// rather than write unvalidated rows.
func financeProductNames(ctx context.Context, client *financeclient.Client) map[int64]string {
	ids := make([]int64, 0, len(productConfigSeeds))
	for _, s := range productConfigSeeds {
		ids = append(ids, s.productSysID)
	}
	products, err := client.BatchGetProducts(ctx, ids)
	if err != nil {
		log.Warn().Err(err).Msg("product config: finance product lookup failed")
		return nil
	}
	names := make(map[int64]string, len(products))
	for _, p := range products {
		if !p.GetIsActive() {
			continue
		}
		names[p.GetProductSysId()] = p.GetProductName()
	}
	return names
}

// deriveDenier extracts the denier from a product name: the first numeric token,
// which by convention leads the yarn spec (POY 300/96/... -> 300,
// TTY 480/216-120/... -> 480). Returns nil when no leading number is present so
// the column stays NULL rather than carrying a fabricated denier.
func deriveDenier(productName string) *float64 {
	digits := make([]rune, 0, 8)
	for _, r := range productName {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
			continue
		}
		if len(digits) > 0 {
			break
		}
	}
	if len(digits) == 0 {
		return nil
	}
	v, err := strconv.ParseFloat(string(digits), 64)
	if err != nil || v <= 0 {
		return nil
	}
	return &v
}

// tally applies an insert result to the running inserted/skipped counters,
// logging any error. A zero-rows result counts as skipped (already present).
func tally(res sql.Result, err error, inserted, skipped int, label string) (int, int) {
	if err != nil {
		log.Error().Err(err).Str("seed", label).Msg("Failed to insert seed")
		return inserted, skipped
	}
	affected, err := res.RowsAffected()
	if err != nil {
		log.Error().Err(err).Str("seed", label).Msg("Failed to read rows affected")
		return inserted, skipped
	}
	if affected == 0 {
		log.Debug().Str("seed", label).Msg("Already exists, skipping")
		return inserted, skipped + 1
	}
	log.Info().Str("seed", label).Msg("Inserted")
	return inserted + 1, skipped
}

func report(table string, inserted, skipped, total int) {
	fmt.Printf("\n%s seeding completed! inserted=%d skipped=%d total=%d\n", table, inserted, skipped, total)
}
