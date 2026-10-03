-- +goose Up
-- One load state per order line per route. A loader submits a load count for a
-- line; a retry or correction must update that record, not append a duplicate.
-- Without this, two submissions for the same line would create two load_item
-- rows for one order_item on one route, and "the route's load state" would be
-- ambiguous. This is the plane the loading domain's idempotency and its
-- concurrent-submission guarantee rest on.
--
-- The existing CHECK (loaded_qty + damaged_qty + missing_qty <= ordered_qty) is
-- kept: it is the database backstop, while the service enforces the equality
-- the business contract states (loaded + damaged + missing = ordered).
CREATE UNIQUE INDEX IF NOT EXISTS load_item_route_order_item_key
    ON load_item (route_id, order_item_id);

-- +goose Down
DROP INDEX IF EXISTS load_item_route_order_item_key;
