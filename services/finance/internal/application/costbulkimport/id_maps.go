// Package costbulkimport handles async bulk import of 6-sheet Excel files
// containing product master and routing data from a legacy Oracle system.
package costbulkimport

import "github.com/google/uuid"

// ImportMaps holds in-memory ID lookup maps built during import processing.
// Pre-loaded maps (ParamMap, ProductTypeMap) are populated once before processing
// begins. The remaining maps are populated sheet-by-sheet as rows are inserted.
type ImportMaps struct {
	// ParamMap maps param_code to mst_parameter.id (UUID).
	// Pre-loaded from DB before processing.
	ParamMap map[string]uuid.UUID
	// ProductTypeMap maps type_code to cost_product_type.cpt_type_id (int32).
	// Pre-loaded from DB before processing.
	ProductTypeMap map[string]int32
	// ProductMap maps legacy_oracle_sys_id to cost_product_master.cpm_product_sys_id (int64).
	// Populated during Sheet 1 (product_master) processing.
	ProductMap map[string]int64
	// RouteHeadMap maps legacy_oracle_sys_id to cost_route_head.crh_head_id (int64).
	// Populated during Sheet 4 (route_head) processing.
	RouteHeadMap map[string]int64
	// RouteSeqMap maps "legacySysId:level:seq" composite key to cost_route_seq.crs_seq_id (int64).
	// Populated during Sheet 5 (route_sequences) processing.
	RouteSeqMap map[string]int64
}

// NewImportMaps returns an initialized ImportMaps with empty maps ready for use.
func NewImportMaps() *ImportMaps {
	return &ImportMaps{
		ParamMap:       make(map[string]uuid.UUID),
		ProductTypeMap: make(map[string]int32),
		ProductMap:     make(map[string]int64),
		RouteHeadMap:   make(map[string]int64),
		RouteSeqMap:    make(map[string]int64),
	}
}
