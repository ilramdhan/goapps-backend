// Package costbulkimport handles async bulk import of 6-sheet Excel files
// containing product master and routing data from a legacy Oracle system.
package costbulkimport

import "github.com/google/uuid"

// ImportMaps holds in-memory ID lookup maps built during import processing.
// Pre-loaded maps (ParamMap, ProductTypeMap, RmGroupMap) are populated once before
// processing begins. The remaining maps are populated sheet-by-sheet as rows are inserted.
type ImportMaps struct {
	// ParamMap maps param_code → mst_parameter UUID. Pre-loaded from DB.
	ParamMap map[string]uuid.UUID
	// ProductTypeMap maps type_code → cpt_type_id (int32). Pre-loaded from DB.
	ProductTypeMap map[string]int32
	// RmGroupMap is the set of active RM group codes from cst_rm_group_head.
	// Pre-loaded from DB. Used to validate rm_group_code in route_rms.
	RmGroupMap map[string]bool
	// ParamLookupMap maps param_code → lookup_master_code for MASTER_LOOKUP params.
	// Pre-loaded from DB. Used to look up which master table to validate against.
	ParamLookupMap map[string]string
	// MasterLookupValues maps lookup_master_code → set of valid code values.
	// Pre-loaded from DB. Used to validate MASTER_LOOKUP param values during import.
	MasterLookupValues map[string]map[string]bool
	// ProductMap maps legacy_oracle_sys_id → cpm_product_sys_id (int64).
	// Populated during Sheet 1 (product_master) processing.
	ProductMap map[string]int64
	// InsertedProductSysIDs records product_sys_ids newly INSERTED (not updated)
	// during Sheet 1. Used to roll back the write phase if a later sheet fails.
	InsertedProductSysIDs []int64
	// RouteHeadMap maps legacy_oracle_sys_id → crh_head_id (int64).
	// Populated during Sheet 4 (route_head) processing.
	RouteHeadMap map[string]int64
	// RouteSeqMap maps "legacySysId:level:seq" composite key → crs_seq_id (int64).
	// Populated during Sheet 5 (route_sequences) processing.
	RouteSeqMap map[string]int64
}

// NewImportMaps returns an initialized ImportMaps with empty maps ready for use.
func NewImportMaps() *ImportMaps {
	return &ImportMaps{
		ParamMap:           make(map[string]uuid.UUID),
		ProductTypeMap:     make(map[string]int32),
		RmGroupMap:         make(map[string]bool),
		ParamLookupMap:     make(map[string]string),
		MasterLookupValues: make(map[string]map[string]bool),
		ProductMap:         make(map[string]int64),
		RouteHeadMap:       make(map[string]int64),
		RouteSeqMap:        make(map[string]int64),
	}
}
