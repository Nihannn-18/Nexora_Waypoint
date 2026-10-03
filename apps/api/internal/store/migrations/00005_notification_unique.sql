-- +goose Up
-- One notification per (recipient, type, reference). The reference is derived
-- from the originating business event (a delivery client_event_id, a load
-- line's route/order_item identity, ...), so a retried event cannot create a
-- duplicate notification. Creation uses ON CONFLICT DO NOTHING against this
-- index, which is the database guarantee rather than call-site bookkeeping.
--
-- reference is nullable in the schema; a NULL reference never conflicts, so the
-- index only constrains notifications that carry a reference. That matches the
-- intent: every operational notification we create today derives from an event
-- identity and carries a reference.
CREATE UNIQUE INDEX IF NOT EXISTS notification_recipient_type_ref_key
    ON notification (user_id, type, reference);

-- +goose Down
DROP INDEX IF EXISTS notification_recipient_type_ref_key;
