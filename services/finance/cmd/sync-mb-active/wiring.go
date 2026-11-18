package main

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	appmbspin "github.com/mutugading/goapps-backend/services/finance/internal/application/mbspin"
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

// buildReporter wires the read-only snapshot and product readers to the configured DB.
func buildReporter() (*mbactivesync.Reporter, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return nil, nil, fmt.Errorf("connect db: %w", err)
	}
	rep := mbactivesync.NewReporter(postgres.NewMBActiveSyncSnapshotReader(db), postgres.NewMBActiveSyncReportReader(db))
	return rep, func() { logClose("database", db) }, nil
}

func runReport(ctx context.Context, o options) error {
	plan, err := readReportPlan(o.planRows, o.planHeads)
	if err != nil {
		return err
	}
	rep, cleanup, err := buildReporter()
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := rep.Run(ctx, plan)
	if err != nil {
		return err
	}
	return writeReport(res, o)
}

// spinDuplicator adapts the existing duplicate-spin use case (mbspin.DuplicateHandler,
// no preview recalc) to mbactivesync.SpinDuplicator (design part 2 §5.4 tx 1).
type spinDuplicator struct {
	h *appmbspin.DuplicateHandler
}

func (d spinDuplicator) Duplicate(ctx context.Context, req mbactivesync.DuplicateSpinRequest) (uuid.UUID, error) {
	res, err := d.h.Handle(ctx, appmbspin.DuplicateCommand{
		SourceSpinID: req.SourceSpinID, MgtName: req.MgtName, Denier: req.Denier, Filament: req.Filament,
		FgProductID: req.FgProductID, ActorUserID: req.Actor,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return res.Output.NewSpinID, nil
}

// newWriter builds the apply writer with the duplicate use case wired in.
func newWriter(db *postgres.DB) *postgres.MBActiveSyncWriter {
	dup := spinDuplicator{h: appmbspin.NewDuplicateHandler(postgres.NewMBSpinRepository(db), nil)}
	return postgres.NewMBActiveSyncWriter(db).WithDuplicator(dup)
}

// newHeadWorkflow wires mbactivesync.HeadWorkflow over the EXISTING Submit/Validate use
// cases exactly as the gRPC MB head handler builds them (with composition gate) — design
// part 2 §5.5. No notifier / lookup-sync trigger: the one-shot apply reports instead.
func newHeadWorkflow(db *postgres.DB) *mbactivesync.HandlerHeadWorkflow {
	compositionRepo := postgres.NewMBCompositionRepository(db)
	headRepo := postgres.NewMBHeadRepository(db, compositionRepo)
	paramRepo := postgres.NewMBParamRepository(db)
	return mbactivesync.NewHandlerHeadWorkflow(headRepo, postgres.NewMBSpinRepository(db),
		appmbhead.NewSubmitHandlerWithComposition(headRepo, compositionRepo),
		appmbhead.NewValidateHandlerWithComposition(headRepo, paramRepo, compositionRepo))
}
