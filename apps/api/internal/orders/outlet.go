package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGOutletReader reads outlets for the order intake. It is the production
// OutletReader, kept here rather than in a full outlet domain package because
// the only consumer so far is order creation.
type PGOutletReader struct {
	pool *pgxpool.Pool
}

// NewPGOutletReader builds an outlet reader over the given pool.
func NewPGOutletReader(pool *pgxpool.Pool) *PGOutletReader {
	return &PGOutletReader{pool: pool}
}

// GetOutlet implements OutletReader.
func (r *PGOutletReader) GetOutlet(ctx context.Context, outletID string) (Outlet, error) {
	var o Outlet
	err := r.pool.QueryRow(ctx, `SELECT outlet_id, brand, depot_id FROM outlet WHERE outlet_id = $1`, outletID).
		Scan(&o.OutletID, &o.Brand, &o.DepotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Outlet{}, fmt.Errorf("%w: %s", ErrNotFound, outletID)
	}
	if err != nil {
		return Outlet{}, fmt.Errorf("get outlet: %w", err)
	}
	return o, nil
}
