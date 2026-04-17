// Package syncdata provides domain types for Oracle-to-PostgreSQL data synchronization.
package syncdata

import (
	"time"

	"github.com/google/uuid"
)

// ItemConsStockPO represents an item consumption, stock, and purchase order record.
// Source: Oracle MGTDAT.MGT_ITEM_CONS_STK_PO.
type ItemConsStockPO struct {
	// Primary key (composite).
	Period    string
	ItemCode  string
	GradeCode string

	// Descriptive fields.
	GradeName string
	ItemName  string
	UOM       string

	// Consumption.
	ConsQty  *float64
	ConsVal  *float64
	ConsRate *float64

	// Stores.
	StoresQty  *float64
	StoresVal  *float64
	StoresRate *float64

	// Department.
	DeptQty  *float64
	DeptVal  *float64
	DeptRate *float64

	// Last PO 1.
	LastPOQty1  *float64
	LastPOVal1  *float64
	LastPORate1 *float64
	LastPODt1   *time.Time

	// Last PO 2.
	LastPOQty2  *float64
	LastPOVal2  *float64
	LastPORate2 *float64
	LastPODt2   *time.Time

	// Last PO 3.
	LastPOQty3  *float64
	LastPOVal3  *float64
	LastPORate3 *float64
	LastPODt3   *time.Time

	// Sync metadata (PostgreSQL only).
	SyncedAt    *time.Time
	SyncedByJob *uuid.UUID
}

// UpsertResult holds statistics from a batch upsert operation.
type UpsertResult struct {
	TotalRows int
	Inserted  int
	Updated   int
}
