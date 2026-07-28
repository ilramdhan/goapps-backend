package commonlot

import "context"

// Filter selects and paginates common lots.
type Filter struct {
	Page      int32
	PageSize  int32
	Search    string
	ItemCode  string
	SortBy    string
	SortOrder string
}

// Repository is the persistence contract for common lots.
type Repository interface {
	// Create inserts a common lot and its components in one transaction, setting
	// the generated id on the aggregate. A duplicate lot number yields
	// ErrAlreadyExists.
	Create(ctx context.Context, lot *CommonLot) error
	// GetByID loads a common lot with its components, or ErrNotFound.
	GetByID(ctx context.Context, id int64) (*CommonLot, error)
	// List returns common lots matching the filter plus the total count.
	List(ctx context.Context, filter Filter) ([]*CommonLot, int64, error)
}
