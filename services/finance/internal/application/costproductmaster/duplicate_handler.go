package costproductmaster

import (
	"context"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// DuplicateCommand input for F2 (duplicate a single product, optionally with its params/values).
type DuplicateCommand struct {
	ProductSysID  int64
	NewCodePrefix string
	CopyParams    bool
	ActorUserID   string
}

// DuplicateHandler clones a product master row, and optionally its CAPP/CPP rows.
type DuplicateHandler struct{ repo domain.Repository }

// NewDuplicateHandler constructs a DuplicateHandler.
func NewDuplicateHandler(r domain.Repository) *DuplicateHandler { return &DuplicateHandler{repo: r} }

// Handle validates the request and delegates to the repository's transactional clone.
func (h *DuplicateHandler) Handle(ctx context.Context, cmd DuplicateCommand) (domain.DuplicateOutput, error) {
	if cmd.ProductSysID <= 0 {
		return domain.DuplicateOutput{}, domain.ErrNotFound
	}
	return h.repo.DuplicateProduct(ctx, domain.DuplicateInput{
		ProductSysID:  cmd.ProductSysID,
		NewCodePrefix: cmd.NewCodePrefix,
		CopyParams:    cmd.CopyParams,
		ActorUserID:   cmd.ActorUserID,
	})
}
