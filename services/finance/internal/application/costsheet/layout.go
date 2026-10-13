package costsheet

import (
	"encoding/json"
	"strings"
)

// ExportLayout selects which sheets a product cost sheet export workbook holds.
// It is persisted in the job params JSON (key "layout") and carried on the
// RabbitMQ message, so it is a plain string rather than the proto enum.
type ExportLayout string

const (
	// LayoutCombined is the original workbook: the flat "all data" sheet plus
	// one vertical sheet per product, at most maxExportProducts per file. It is
	// the default for an unset layout and for jobs queued before the field
	// existed.
	LayoutCombined ExportLayout = "combined"
	// LayoutHorizontal renders only the flat "all data" sheet into one file,
	// without the maxExportProducts chunking.
	LayoutHorizontal ExportLayout = "horizontal"
	// LayoutVertical renders only the per-product vertical sheets, chunked at
	// maxExportProducts per file like the combined layout.
	LayoutVertical ExportLayout = "vertical"
)

// layoutParamKey is the job params JSON key holding the layout.
const layoutParamKey = "layout"

// ParseExportLayout normalizes a raw layout string. Anything unknown or empty
// maps to LayoutCombined, so old messages and params keep today's behavior.
func ParseExportLayout(raw string) ExportLayout {
	switch ExportLayout(strings.ToLower(strings.TrimSpace(raw))) {
	case LayoutHorizontal:
		return LayoutHorizontal
	case LayoutVertical:
		return LayoutVertical
	default:
		return LayoutCombined
	}
}

// LayoutFromParams reads the layout out of a job's params JSON. Params that
// are empty, undecodable, or written before the field existed yield
// LayoutCombined.
func LayoutFromParams(raw json.RawMessage) ExportLayout {
	if len(raw) == 0 {
		return LayoutCombined
	}
	var params struct {
		Layout string `json:"layout"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return LayoutCombined
	}
	return ParseExportLayout(params.Layout)
}
