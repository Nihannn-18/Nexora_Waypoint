package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository reads catalogue items. It is an interface so the service can be
// tested without a database; the only production implementation is PGRepository.
//
// Catalogue writes are out of scope for this foundation: the catalogue is
// reference data seeded from an authoritative source, not edited through the
// API. Adding a write method later belongs with the admin surface that needs it.
type Repository interface {
	// GetByID returns the item with itemID, or ErrNotFound.
	GetByID(ctx context.Context, itemID string) (Item, error)
	// GetBySKU returns the item with the given SKU, or ErrNotFound.
	GetBySKU(ctx context.Context, sku string) (Item, error)
	// List returns items matching filter, ordered by SKU for determinism.
	List(ctx context.Context, filter Filter) ([]Item, error)
}

// PGRepository is the PostgreSQL-backed Repository. It uses hand-written SQL
// over the `item` table and never leaks persistence structs to callers.
type PGRepository struct {
	pool *pgxpool.Pool
}

// NewPGRepository builds a repository over the given pool.
func NewPGRepository(pool *pgxpool.Pool) *PGRepository {
	return &PGRepository{pool: pool}
}

// itemColumns is the projection shared by every read, so the scan order below
// and the column list cannot drift apart.
const itemColumns = `item_id, sku, name, brand, COALESCE(category, ''), unit_weight_kg, unit_volume_m3, temperature_requirement`

// GetByID implements Repository.
func (r *PGRepository) GetByID(ctx context.Context, itemID string) (Item, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+itemColumns+` FROM item WHERE item_id::text = $1`, itemID)
	item, err := scanItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, fmt.Errorf("%w: %s", ErrNotFound, itemID)
	}
	if err != nil {
		return Item{}, fmt.Errorf("get item by id: %w", err)
	}
	return item, nil
}

// GetBySKU implements Repository.
func (r *PGRepository) GetBySKU(ctx context.Context, sku string) (Item, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+itemColumns+` FROM item WHERE sku = $1`, sku)
	item, err := scanItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Item{}, fmt.Errorf("%w: sku %s", ErrNotFound, sku)
	}
	if err != nil {
		return Item{}, fmt.Errorf("get item by sku: %w", err)
	}
	return item, nil
}

// List implements Repository. Filters are applied with parameterised SQL; an
// empty filter lists the whole catalogue.
func (r *PGRepository) List(ctx context.Context, filter Filter) ([]Item, error) {
	var (
		where []string
		args  []any
	)
	if filter.Brand != "" {
		args = append(args, string(filter.Brand))
		where = append(where, fmt.Sprintf("brand = $%d", len(args)))
	}
	if filter.Temperature != "" {
		args = append(args, string(filter.Temperature))
		where = append(where, fmt.Sprintf("temperature_requirement = $%d", len(args)))
	}
	if s := strings.TrimSpace(filter.Search); s != "" {
		args = append(args, "%"+s+"%")
		where = append(where, fmt.Sprintf("(sku ILIKE $%d OR name ILIKE $%d)", len(args), len(args)))
	}

	query := `SELECT ` + itemColumns + ` FROM item`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	// SKU ordering makes the listing deterministic for tests and for a stable UI.
	query += " ORDER BY sku"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items: %w", err)
	}
	return items, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(s rowScanner) (Item, error) {
	var item Item
	err := s.Scan(
		&item.ItemID,
		&item.SKU,
		&item.Name,
		&item.Brand,
		&item.Category,
		&item.UnitWeightKg,
		&item.UnitVolumeM3,
		&item.TemperatureRequirement,
	)
	if err != nil {
		return Item{}, err
	}
	return item, nil
}
