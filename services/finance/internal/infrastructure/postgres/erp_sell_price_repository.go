package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// ErpSellPriceRepository implements erprule.SellPriceRepository over
// cst_erp_sell_price (migration 000547; design Part 1 §4.1; plan-03 P2-T3).
// The price is bound and scanned as text (no float round trip).
type ErpSellPriceRepository struct{ db *DB }

// NewErpSellPriceRepository constructs the repository.
func NewErpSellPriceRepository(db *DB) *ErpSellPriceRepository {
	return &ErpSellPriceRepository{db: db}
}

var _ erprule.SellPriceRepository = (*ErpSellPriceRepository)(nil)

const sellPriceColumns = `
	cesp_basis, cesp_price::text, cesp_is_active, created_at, created_by,
	updated_at, COALESCE(updated_by, '')`

// upsertSellPriceSQL keeps created_at/by of an existing row. On conflict the
// update stamp falls back to the acting user and NOW() when the domain value
// carries none (a NewSellPrice upserted over an existing basis).
const upsertSellPriceSQL = `
INSERT INTO cst_erp_sell_price
       (cesp_basis, cesp_price, cesp_is_active, created_at, created_by, updated_at, updated_by)
VALUES ($1, $2::numeric, $3, $4, $5, $6, $7)
ON CONFLICT (cesp_basis) DO UPDATE
   SET cesp_price     = EXCLUDED.cesp_price,
       cesp_is_active = EXCLUDED.cesp_is_active,
       updated_at     = COALESCE(EXCLUDED.updated_at, NOW()),
       updated_by     = COALESCE(EXCLUDED.updated_by, EXCLUDED.created_by)`

// Get returns the price row (active or not) for a basis, or
// erprule.ErrSellPriceNotFound.
func (r *ErpSellPriceRepository) Get(ctx context.Context, basis erprule.Basis) (*erprule.SellPrice, error) {
	q := `SELECT ` + sellPriceColumns + ` FROM cst_erp_sell_price WHERE cesp_basis = $1`
	out, err := scanSellPrice(r.db.QueryRowContext(ctx, q, basis.String()))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, erprule.ErrSellPriceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("erp sell price get: %w", err)
	}
	return out, nil
}

// List returns every sell price ordered by basis.
func (r *ErpSellPriceRepository) List(ctx context.Context, includeInactive bool) (out []*erprule.SellPrice, err error) {
	q := `SELECT ` + sellPriceColumns + ` FROM cst_erp_sell_price`
	if !includeInactive {
		q += ` WHERE cesp_is_active`
	}
	q += ` ORDER BY cesp_basis`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("erp sell price list: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("erp sell price list close: %w", cerr)
		}
	}()
	for rows.Next() {
		p, serr := scanSellPrice(rows)
		if serr != nil {
			return nil, fmt.Errorf("erp sell price list scan: %w", serr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp sell price list rows: %w", err)
	}
	return out, nil
}

// Upsert inserts or updates the price row for its basis.
func (r *ErpSellPriceRepository) Upsert(ctx context.Context, price *erprule.SellPrice) error {
	if price == nil {
		return errors.New("erp sell price upsert: nil price")
	}
	var updatedAt any
	if t := price.UpdatedAt(); t != nil {
		updatedAt = *t
	}
	var updatedBy any
	if u := price.UpdatedBy(); u != "" {
		updatedBy = u
	}
	if _, err := r.db.ExecContext(ctx, upsertSellPriceSQL,
		price.Basis().String(), price.Price().String(), price.IsActive(),
		price.CreatedAt(), price.CreatedBy(), updatedAt, updatedBy); err != nil {
		return fmt.Errorf("erp sell price upsert: %w", err)
	}
	return nil
}

func scanSellPrice(s rowScanner) (*erprule.SellPrice, error) {
	var (
		basis, priceText     string
		isActive             bool
		createdAt            time.Time
		createdBy, updatedBy string
		updatedAt            sql.NullTime
	)
	if err := s.Scan(&basis, &priceText, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy); err != nil {
		return nil, err
	}
	price, err := decimal.NewFromString(priceText)
	if err != nil {
		return nil, fmt.Errorf("parse cesp_price %q: %w", priceText, err)
	}
	return erprule.ReconstructSellPrice(erprule.Basis(basis), price, isActive,
		createdAt, createdBy, nullTimePtr(updatedAt), updatedBy), nil
}
