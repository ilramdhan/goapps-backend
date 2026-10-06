package erpintegration

import (
	"context"
	"errors"
)

// MasterItem is one OM_ITEM row as read (SELECT only) from Oracle.
type MasterItem struct {
	Code   string
	Name   string
	Active bool
}

// MasterGrade is one OM_GRADE_CODE_1 row as read (SELECT only) from Oracle.
type MasterGrade struct {
	Code   string
	Name   string
	Active bool
}

// MasterReader reads the ERP master tables through the read-only connection.
type MasterReader interface {
	ListItems(ctx context.Context) ([]MasterItem, error)
	ListGrades(ctx context.Context) ([]MasterGrade, error)
}

// UpsertCounts summarizes a replica upsert.
type UpsertCounts struct {
	Read      int64
	Inserted  int64
	Updated   int64
	Unchanged int64
}

// GradeGroupApplyReport is the outcome of re-applying the grade-group seed
// (P0-T15b). Only NULL groups are filled; a group set by Costing is never
// overwritten.
type GradeGroupApplyReport struct {
	SeedRows       int64
	AppliedNow     int64
	AlreadyGrouped int64
	MissingCodes   []string
}

// MasterRepository persists the PostgreSQL replicas cost_erp_item and
// cost_erp_grade. Upserts touch replica columns only and never
// ceg_grade_group.
type MasterRepository interface {
	UpsertItems(ctx context.Context, items []MasterItem) (UpsertCounts, error)
	UpsertGrades(ctx context.Context, grades []MasterGrade) (UpsertCounts, error)
	ApplyGradeGroupSeed(ctx context.Context) (GradeGroupApplyReport, error)
}

// ErrMasterSyncNotConfigured is returned when the master sync has no reader
// or repository (for example, no Oracle read connection configured).
var ErrMasterSyncNotConfigured = errors.New("erpintegration: ERP master sync not configured")

// ErrInvalidMasterSubtype is returned for an unknown erp_master_sync subtype.
var ErrInvalidMasterSubtype = errors.New("erpintegration: invalid erp_master_sync subtype")
