-- +goose Up
-- A loader's shortfall photograph attaches to a single order line, mirroring
-- delivery_event.pod_photo for the driver. The value is a server-generated
-- media key (see apps/api/internal/media), not a client-supplied path.
ALTER TABLE load_item ADD COLUMN photo_ref TEXT;

-- +goose Down
ALTER TABLE load_item DROP COLUMN photo_ref;
