package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

type closer interface{ Close() error }

func logClose(name string, c closer) {
	if err := c.Close(); err != nil {
		log.Printf("WARN: failed to close %s: %v", name, err)
	}
}

// buildSnapshotReader wires the read-only snapshot reader to the configured DB.
// The returned cleanup closes the connection.
func buildSnapshotReader() (mbactivesync.SnapshotReader, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("connect db: %w", err)
	}
	return postgres.NewMBActiveSyncSnapshotReader(db), func() { logClose("database", db) }, nil
}

func runPlan(ctx context.Context, o options) error {
	rows, err := mbactivesync.ParseFile(o.file)
	if err != nil {
		return fmt.Errorf("parse file: %w", err)
	}
	reader, cleanup, err := buildSnapshotReader()
	if err != nil {
		return err
	}
	defer cleanup()
	return planAndWrite(ctx, mbactivesync.NewPlanner(reader), rows, o.outDir)
}
