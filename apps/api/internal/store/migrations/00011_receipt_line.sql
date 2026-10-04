-- +goose Up
-- S-06 goods received note (GRN): the store manager's line-by-line receipt.
--
-- The receipt table from 00001 was never written, and its shape could not carry
-- a GRN: one status with one free-text issue type, no per-line quantities. The
-- design requires expected vs received **per order line** with its condition, so
-- the line detail moves to receipt_line and the header keeps who, when and an
-- optional note.
--
--   * One GRN per order. A second submission is a conflict, not a second
--     receipt, so a double click or a retried request cannot record twice.
--   * Status follows the shared contract (RECEIPT_STATUSES): RECEIVED when every
--     line arrived as expected, RECEIVED_WITH_ISSUE otherwise. The old
--     PENDING/CONFIRMED/ISSUE vocabulary had no writer and no reader.
--   * issue_type and proof are dropped: issues are per line now, and the proof
--     attached to a GRN is the driver's POD, which stays on delivery_event.
--
-- receipt_line stores the facts of the receipt: what was expected at the time
-- (snapshotted, because the loader's count is what the store was told to
-- expect), what arrived in good condition and what arrived damaged. Short is
-- derived on read (expected - received - damaged) and is never stored.
ALTER TABLE receipt DROP CONSTRAINT IF EXISTS receipt_status_check;
ALTER TABLE receipt ADD CONSTRAINT receipt_status_check
    CHECK (status IN ('RECEIVED', 'RECEIVED_WITH_ISSUE'));
ALTER TABLE receipt DROP COLUMN IF EXISTS issue_type;
ALTER TABLE receipt DROP COLUMN IF EXISTS proof;
ALTER TABLE receipt ALTER COLUMN received_at SET NOT NULL;
DROP INDEX IF EXISTS receipt_order_idx;
CREATE UNIQUE INDEX receipt_order_key ON receipt (order_id);

CREATE TABLE receipt_line (
    receipt_line_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id      UUID NOT NULL REFERENCES receipt (receipt_id) ON DELETE CASCADE,
    order_item_id   UUID NOT NULL REFERENCES order_item (order_item_id),
    expected_qty    INTEGER NOT NULL CHECK (expected_qty >= 0),
    received_qty    INTEGER NOT NULL CHECK (received_qty >= 0),
    damaged_qty     INTEGER NOT NULL DEFAULT 0 CHECK (damaged_qty >= 0),
    UNIQUE (receipt_id, order_item_id),
    CHECK (received_qty + damaged_qty <= expected_qty)
);

-- +goose Down
DROP TABLE IF EXISTS receipt_line;
DROP INDEX IF EXISTS receipt_order_key;
CREATE INDEX IF NOT EXISTS receipt_order_idx ON receipt (order_id);
ALTER TABLE receipt ALTER COLUMN received_at DROP NOT NULL;
ALTER TABLE receipt ADD COLUMN IF NOT EXISTS proof TEXT;
ALTER TABLE receipt ADD COLUMN IF NOT EXISTS issue_type TEXT;
ALTER TABLE receipt DROP CONSTRAINT IF EXISTS receipt_status_check;
ALTER TABLE receipt ADD CONSTRAINT receipt_status_check
    CHECK (status IN ('PENDING', 'CONFIRMED', 'ISSUE'));
