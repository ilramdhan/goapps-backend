// Package costroute holds the persisted routing DAG aggregate
// (cost_route_head + cost_route_seq + cost_route_rm).
//
// Full domain + behavior methods land in S7.16b. This file pins the
// repository contract that PromoteHandler and the gRPC handler depend on.
package costroute

import (
	"context"
	"errors"
	"time"
)

// Status values for cost_route_head.crh_routing_status.
const (
	StatusDraft    = "DRAFT"
	StatusComplete = "COMPLETE"
	StatusLocked   = "LOCKED"
)

// RmRefType discriminator for cost_route_rm.
const (
	RmTypeProduct = "PRODUCT"
	RmTypeItem    = "ITEM"
	RmTypeGroup   = "GROUP"
)

// Head mirrors cost_route_head columns.
type Head struct {
	HeadID              int64
	ProductSysID        int64
	ProductCode         string
	ProductName         string
	RoutingStatus       string
	Version             int32
	PromotedFromDraftID int64
	CylTypeID           int32
	Notes               string
	CreatedAt           time.Time
	CreatedBy           string
	UpdatedAt           time.Time
	UpdatedBy           string
	// Lock tracking — populated from DB on read, set by Lock/Unlock methods.
	LockedBy   string
	LockedAt   time.Time
	UnlockedBy string
	UnlockedAt time.Time
}

// Seq mirrors cost_route_seq columns.
type Seq struct {
	SeqID          int64
	HeadID         int64
	ProductSysID   int64
	ProductCode    string
	ProductName    string
	RouteLevel     int32
	RouteSeq       int32
	RouteName      string
	RouteItemCode  string
	RouteShadeCode string
	RouteShadeName string
	PositionX      float64
	PositionY      float64
	Rms            []*Rm
}

// Rm mirrors cost_route_rm columns. Exactly one of the three ref columns is
// set, matching RmType.
type Rm struct {
	RmID               int64
	SeqID              int64
	ParentProductSysID int64
	RmType             string
	RmProductSysID     int64
	RmItemCode         string
	RmGroupCode        string
	RouteRmName        string
	RouteRmItemCode    string
	RouteRmShadeCode   string
	RouteRmShadeName   string
	RouteRmRatio       float64
	UomID              int32
	SubType            string
	Notes              string
}

// Graph bundles head + seqs (with rms inline).
type Graph struct {
	Head *Head
	Seqs []*Seq
}

// PromoteInput drives the level-1 seed when a routing draft is promoted.
type PromoteInput struct {
	ProductSysID        int64
	CylTypeID           int32
	PromotedFromDraftID int64
	ActorUserID         string
	// LevelOneRMs are the draft components mapped 1:1 into route_rm rows on the
	// freshly-created level=1 SEQ. Each Rm here must have RmType+ref set; the
	// repo fills HeadID/SeqID after insert.
	LevelOneRMs []*Rm
}

// Sentinel errors.
var (
	ErrNotFound                = errors.New("route not found")
	ErrAlreadyExists           = errors.New("route already exists for product")
	ErrLocked                  = errors.New("route is locked")
	ErrInvalidStatusTransition = errors.New("invalid route status transition")
	// ErrParamIncomplete is returned when locking is attempted with unfilled required params.
	ErrParamIncomplete = errors.New("required params incomplete")
)

// DuplicateInput is the use-case payload for a deep-fork of a route.
type DuplicateInput struct {
	SourceHeadID         int64
	IncludeRouting       bool
	IncludeUpstream      bool
	IncludeApplicability bool
	IncludeValues        bool
	NewCodePrefix        string
	LinkedRequestID      int64 // when >0, atomically set cpr_linked_route_head_id
	ActorUserID          string
}

// DuplicateOutput is the result returned by DuplicateRoute.
type DuplicateOutput struct {
	NewHeadID       int64
	NewProductSysID int64
	NewProductCode  string
}

// LinkedRequest is the read model for ListLinkedRequests.
type LinkedRequest struct {
	RequestID   int64
	RequestNo   string
	Status      string
	ProductTop2 string
	CreatedBy   string
	CreatedAt   time.Time
}

// Filter drives ListHeads.
type Filter struct {
	Search    string
	Status    string
	Page      int32
	PageSize  int32
	SortBy    string
	SortOrder string
}

// HeadUpsertInput is a single row for BulkUpsertHeads.
type HeadUpsertInput struct {
	LegacySysID   string // the legacy_oracle_sys_id used to look up ProductSysID
	ProductSysID  int64
	RoutingStatus string // default "DRAFT" if empty
	Notes         string
}

// HeadUpsertResult reports the outcome for one upserted route head.
type HeadUpsertResult struct {
	LegacySysID string
	HeadID      int64
	WasInserted bool
	Skipped     bool // true when route is LOCKED
}

// SeqUpsertInput is a single row for BulkUpsertSeqs.
type SeqUpsertInput struct {
	HeadLegacySysID  string // used to look up HeadID via RouteHeadMap
	HeadID           int64
	NodeProductSysID int64
	RouteLevel       int32
	RouteSeq         int32
	RouteName        string
	RouteItemCode    string
	RouteShadeCode   string
	RouteShadeName   string
}

// SeqUpsertResult reports the outcome for one upserted route sequence.
type SeqUpsertResult struct {
	LegacySysID string // HeadLegacySysID
	SeqID       int64
	HeadID      int64
	RouteLevel  int32
	RouteSeq    int32
}

// RMInput is a single RM row for BulkReplaceRMs.
type RMInput struct {
	RmType         string // PRODUCT, ITEM, GROUP
	RmProductSysID int64  // set when RmType=PRODUCT
	RmItemCode     string // set when RmType=ITEM
	RmGroupCode    string // set when RmType=GROUP
	Ratio          float64
	RmName         string
	RmShadeCode    string
	RmShadeName    string
	SubType        string
	Notes          string
}

// ExportRouteHead is a flat projection of cost_route_head used for bulk export.
type ExportRouteHead struct {
	HeadID        int64
	ProductSysID  int64
	RoutingStatus string
	Notes         string
}

// ExportRouteSeq is a flat projection of cost_route_seq used for bulk export.
type ExportRouteSeq struct {
	SeqID          int64
	HeadID         int64
	ProductSysID   int64
	RouteLevel     int32
	RouteSeq       int32
	RouteName      string
	RouteItemCode  string
	RouteShadeCode string
	RouteShadeName string
}

// ExportRouteRM is a flat projection of cost_route_rm used for bulk export.
type ExportRouteRM struct {
	SeqID          int64
	HeadID         int64
	RouteLevel     int32
	RouteSeq       int32
	RmType         string
	RmProductSysID int64
	RmItemCode     string
	RmGroupCode    string
	Ratio          float64
	RmName         string
	RmShadeCode    string
	RmShadeName    string
	SubType        string
	Notes          string
}

// Repository persists CostRoute aggregates.
type Repository interface {
	// PromoteFromDraft creates a new cost_route_head + a level-1 SEQ producing
	// the FG product + one route_rm per LevelOneRMs entry. Returns the new
	// head_id.
	PromoteFromDraft(ctx context.Context, in PromoteInput) (headID int64, err error)
	// GetActiveByProduct returns the non-LOCKED head for a product, or
	// ErrNotFound. Used by promote to surface a friendly conflict before the
	// DB unique violation fires.
	GetActiveByProduct(ctx context.Context, productSysID int64) (*Head, error)
	// GetHead returns the head row (or ErrNotFound).
	GetHead(ctx context.Context, headID int64) (*Head, error)
	// GetGraph returns the full graph (head + seqs with rms inline) for headID.
	GetGraph(ctx context.Context, headID int64) (*Graph, error)
	// SaveGraph performs a bulk diff+upsert against the persisted state:
	//   - seqs missing from payload are DELETED (cascade their RMs);
	//   - seqs with seq_id=0 are INSERTED;
	//   - seqs with seq_id>0 are UPDATED in place;
	//   - same logic for rms within each seq.
	// Caller is expected to have already passed Graph.ValidateLevels().
	// Returns the fresh graph (with newly-generated IDs filled in).
	SaveGraph(ctx context.Context, headID int64, graph *Graph, actor string) (*Graph, error)
	// SaveHead persists status transitions + audit columns. Used by
	// MarkComplete/Lock/Unlock handlers.
	SaveHead(ctx context.Context, head *Head, actor string) error
	// DeleteHead soft-deletes the head (cascade through seq/rm via DB FK).
	DeleteHead(ctx context.Context, headID int64, actor string) error
	// ListHeads applies a search/filter and returns paginated heads.
	ListHeads(ctx context.Context, f Filter) (rows []*Head, total int64, err error)
	// DuplicateRoute deep-forks a route per the requested toggles, all in one tx.
	DuplicateRoute(ctx context.Context, in DuplicateInput) (DuplicateOutput, error)
	// ListLinkedRequests returns requests linking to this route head.
	ListLinkedRequests(ctx context.Context, headID int64) ([]LinkedRequest, error)
	// BulkUpsertHeads upserts route head rows by (crh_product_sys_id).
	// Rows where crh_routing_status = 'LOCKED' are skipped and returned with Skipped=true.
	BulkUpsertHeads(ctx context.Context, items []HeadUpsertInput, actor string) ([]HeadUpsertResult, error)
	// BulkUpsertSeqs upserts route sequence rows by (crs_head_id, crs_route_level, crs_route_seq).
	BulkUpsertSeqs(ctx context.Context, items []SeqUpsertInput, actor string) ([]SeqUpsertResult, error)
	// BulkReplaceRMs deletes all existing RMs for seqID and re-inserts the given rms.
	BulkReplaceRMs(ctx context.Context, seqID int64, rms []RMInput, actor string) error
	// ListAllHeadsForExport returns all non-deleted route heads for export, optionally
	// filtered to the given product sys IDs. An empty productSysIDs slice returns all heads.
	ListAllHeadsForExport(ctx context.Context, productSysIDs []int64) ([]ExportRouteHead, error)
	// ListAllSeqsForExport returns all non-deleted route seqs for the given head IDs.
	ListAllSeqsForExport(ctx context.Context, headIDs []int64) ([]ExportRouteSeq, error)
	// ListAllRMsForExport returns all route RMs for the given seq IDs.
	ListAllRMsForExport(ctx context.Context, seqIDs []int64) ([]ExportRouteRM, error)
}
