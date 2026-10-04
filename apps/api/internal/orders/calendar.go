package orders

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OperatingDayReader answers the one calendar question the 16:00 cutoff needs:
// which operating day comes next. It reads the authoritative `calendar_day`
// table rather than assuming weekdays, so a holiday or a closed day shifts the
// following run the same way the planner does.
type OperatingDayReader interface {
	// NextOperatingDay returns the first operating day strictly after `from`,
	// normalised to midnight UTC (the DATE convention the calendar uses).
	NextOperatingDay(ctx context.Context, from time.Time) (time.Time, error)
}

// PGOperatingDayReader is the PostgreSQL-backed OperatingDayReader.
type PGOperatingDayReader struct {
	pool *pgxpool.Pool
}

// NewPGOperatingDayReader builds a reader over the given pool.
func NewPGOperatingDayReader(pool *pgxpool.Pool) *PGOperatingDayReader {
	return &PGOperatingDayReader{pool: pool}
}

// NextOperatingDay implements OperatingDayReader.
func (r *PGOperatingDayReader) NextOperatingDay(ctx context.Context, from time.Time) (time.Time, error) {
	if r == nil || r.pool == nil {
		return time.Time{}, fmt.Errorf("orders: calendar is not configured")
	}
	var day time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT date
		FROM calendar_day
		WHERE date > $1 AND is_operating
		ORDER BY date
		LIMIT 1`, from.Format("2006-01-02")).Scan(&day)
	if err != nil {
		return time.Time{}, fmt.Errorf("next operating day after %s: %w", from.Format("2006-01-02"), err)
	}
	return day, nil
}
