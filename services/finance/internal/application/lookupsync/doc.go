// Package lookupsync resolves the values a lookup master record contributes to
// its fill-group child parameters.
//
// Given a lookup master code (MACHINE, INTERMINGLING, PRODUCT_GRADE, MB_HEAD,
// MB_SPIN, BOX_BOBBIN_COST, RM_GROUP_OIL), the selected master key and the
// trigger (source) parameter code, the Resolver loads the master record, reads
// the fill-group children of the trigger parameter and maps each child's
// lookup_source_column to a numeric or text value. The result is plain Go data
// so it can be reused by the interactive Param-tab fill (the YarnLookupFill gRPC
// handler, a thin adapter over this package) and by the master-lookup-sync
// planner, which re-applies master values to products that already reference a
// master record.
//
// The reader maps in readers.go are the single source of truth for which
// lookup_source_column each master can resolve; LookupReaderColumns derives the
// inventory the startup registry-divergence check consumes from them.
package lookupsync
