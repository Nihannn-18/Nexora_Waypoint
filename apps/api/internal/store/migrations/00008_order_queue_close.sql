-- +goose Up
-- order_queue_close records that the dispatcher has frozen the planning queue for
-- one delivery date, depot and brand. Closing is explicit (D-02) and per brand,
-- so the (queue_date, depot_id, brand) triple is the natural key and an upsert
-- makes a repeated close idempotent. The row is the authoritative "queue is
-- closed" fact; the queue read endpoint and the planning start introspect it.
CREATE TABLE order_queue_close (
    queue_date DATE NOT NULL,
    depot_id   UUID NOT NULL REFERENCES depot (depot_id) ON DELETE CASCADE,
    brand      TEXT NOT NULL CHECK (brand IN ('FRESH', 'STYLE', 'TECH')),
    closed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_by  TEXT,
    PRIMARY KEY (queue_date, depot_id, brand)
);

-- +goose Down
DROP TABLE order_queue_close;
