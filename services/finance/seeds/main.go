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
	code        string
	name        string
	category    string
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

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Setup logger
	logger.Setup(cfg.Logger.Level, cfg.Logger.Format, cfg.Logger.PrettyJSON)

	log.Info().Msg("Starting UOM seeder")

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

	// Seed UOMs
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

		// Insert
		_, err = db.ExecContext(ctx,
			`INSERT INTO mst_uom (uom_code, uom_name, uom_category, description, is_active, created_by)
			 VALUES ($1, $2, $3, $4, true, 'seeder')`,
			seed.code, seed.name, seed.category, seed.description,
		)
		if err != nil {
			log.Error().Err(err).Str("code", seed.code).Msg("Failed to insert UOM")
			continue
		}

		log.Info().Str("code", seed.code).Str("name", seed.name).Msg("Inserted UOM")
		inserted++
	}

	fmt.Printf("\n✅ Seeding completed!\n")
	fmt.Printf("   Inserted: %d\n", inserted)
	fmt.Printf("   Skipped:  %d\n", skipped)
	fmt.Printf("   Total:    %d\n", len(uomSeeds))
}
