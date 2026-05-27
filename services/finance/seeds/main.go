// Package main provides the database seeder for finance service.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/pkg/logger"
)

type uomSeed struct {
	code         string
	name         string
	categoryCode string
	description  string
}

type rmCategorySeed struct {
	code        string
	name        string
	description string
}

var uomSeeds = []uomSeed{
	// Weight
	{"KG", "Kilogram", "WEIGHT", "Weight in kilograms"},
	{"GR", "Gram", "WEIGHT", "Weight in grams"},
	{"TON", "Ton", "WEIGHT", "Weight in tons (metric)"},
	{"MG", "Milligram", "WEIGHT", "Weight in milligrams"},
	{"LB", "Pound", "WEIGHT", "Weight in pounds"},
	{"OZ", "Ounce", "WEIGHT", "Weight in ounces"},

	// Length
	{"MTR", "Meter", "LENGTH", "Length in meters"},
	{"CM", "Centimeter", "LENGTH", "Length in centimeters"},
	{"MM", "Millimeter", "LENGTH", "Length in millimeters"},
	{"KM", "Kilometer", "LENGTH", "Length in kilometers"},
	{"INCH", "Inch", "LENGTH", "Length in inches"},
	{"FT", "Feet", "LENGTH", "Length in feet"},
	{"YARD", "Yard", "LENGTH", "Length in yards"},

	// Volume
	{"LTR", "Liter", "VOLUME", "Volume in liters"},
	{"ML", "Milliliter", "VOLUME", "Volume in milliliters"},
	{"GAL", "Gallon", "VOLUME", "Volume in gallons"},
	{"M3", "Cubic Meter", "VOLUME", "Volume in cubic meters"},

	// Quantity
	{"PCS", "Pieces", "QUANTITY", "Count in pieces"},
	{"BOX", "Box", "QUANTITY", "Count in boxes"},
	{"SET", "Set", "QUANTITY", "Count in sets"},
	{"PACK", "Pack", "QUANTITY", "Count in packs"},
	{"UNIT", "Unit", "QUANTITY", "Count in units"},
	{"DOZEN", "Dozen", "QUANTITY", "Count in dozens (12 pieces)"},
	{"ROLL", "Roll", "QUANTITY", "Count in rolls"},
	{"CONE", "Cone", "QUANTITY", "Count in cones (yarn)"},
	{"DRUM", "Drum", "QUANTITY", "Count in drums (container)"},
}

var rmCategorySeeds = []rmCategorySeed{
	{"CHIP", "Chips", "Chip-based raw materials"},
	{"OIL", "Oil", "Oil-based raw materials"},
	{"DYES", "Dyes", "Dye and coloring raw materials"},
	{"CHEM", "Chemicals", "Chemical raw materials"},
	{"FIBER", "Fiber", "Fiber and textile raw materials"},
	{"RESIN", "Resin", "Resin and polymer raw materials"},
	{"SOLV", "Solvent", "Solvent raw materials"},
	{"ADD", "Additives", "Additive and auxiliary raw materials"},
	{"PACK", "Packaging", "Packaging raw materials"},
	{"MISC", "Miscellaneous", "Other raw materials"},
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logger
	logger.Setup(cfg.Logger.Level, cfg.Logger.Format, cfg.Logger.PrettyJSON)

	log.Info().Msg("Starting Finance seeder")

	// Connect to database
	db, err := sql.Open("postgres", cfg.Database.ConnectionString())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Warn().Err(err).Msg("Failed to close database connection")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to ping database")
		return
	}

	// Seed UOMs (uses uom_category_id FK via lookup on mst_uom_category)
	seedUOMs(ctx, db)

	// Seed RM Categories
	seedRMCategories(ctx, db)
}

func seedUOMs(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding UOMs")

	inserted := 0
	skipped := 0

	for _, seed := range uomSeeds {
		// Check if exists
		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM mst_uom WHERE uom_code = $1 AND deleted_at IS NULL)",
			seed.code,
		).Scan(&exists)
		if err != nil {
			log.Error().Err(err).Str("code", seed.code).Msg("Failed to check existence")
			continue
		}

		if exists {
			log.Debug().Str("code", seed.code).Msg("UOM already exists, skipping")
			skipped++
			continue
		}

		// Lookup category ID from mst_uom_category
		var categoryID string
		err = db.QueryRowContext(ctx,
			"SELECT uom_category_id FROM mst_uom_category WHERE category_code = $1 AND deleted_at IS NULL",
			seed.categoryCode,
		).Scan(&categoryID)
		if err != nil {
			log.Error().Err(err).Str("code", seed.code).Str("category", seed.categoryCode).
				Msg("Failed to find UOM category - run migration 000007 first")
			continue
		}

		// Insert with uom_category_id FK
		_, err = db.ExecContext(ctx,
			`INSERT INTO mst_uom (uom_code, uom_name, uom_category_id, description, is_active, created_by)
			 VALUES ($1, $2, $3, $4, true, 'seeder')`,
			seed.code, seed.name, categoryID, seed.description,
		)
		if err != nil {
			log.Error().Err(err).Str("code", seed.code).Msg("Failed to insert UOM")
			continue
		}

		log.Info().Str("code", seed.code).Str("name", seed.name).Msg("Inserted UOM")
		inserted++
	}

	fmt.Printf("\n✅ UOM seeding completed!\n")
	fmt.Printf("   Inserted: %d\n", inserted)
	fmt.Printf("   Skipped:  %d\n", skipped)
	fmt.Printf("   Total:    %d\n", len(uomSeeds))
}

func seedRMCategories(ctx context.Context, db *sql.DB) {
	log.Info().Msg("Seeding RM Categories")

	inserted := 0
	skipped := 0

	for _, seed := range rmCategorySeeds {
		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM mst_rm_category WHERE category_code = $1 AND deleted_at IS NULL)",
			seed.code,
		).Scan(&exists)
		if err != nil {
			log.Error().Err(err).Str("code", seed.code).Msg("Failed to check RM Category existence")
			continue
		}

		if exists {
			log.Debug().Str("code", seed.code).Msg("RM Category already exists, skipping")
			skipped++
			continue
		}

		_, err = db.ExecContext(ctx,
			`INSERT INTO mst_rm_category (category_code, category_name, description, is_active, created_by)
			 VALUES ($1, $2, $3, true, 'seeder')`,
			seed.code, seed.name, seed.description,
		)
		if err != nil {
			log.Error().Err(err).Str("code", seed.code).Msg("Failed to insert RM Category")
			continue
		}

		log.Info().Str("code", seed.code).Str("name", seed.name).Msg("Inserted RM Category")
		inserted++
	}

	fmt.Printf("\n✅ RM Category seeding completed!\n")
	fmt.Printf("   Inserted: %d\n", inserted)
	fmt.Printf("   Skipped:  %d\n", skipped)
	fmt.Printf("   Total:    %d\n", len(rmCategorySeeds))
}
