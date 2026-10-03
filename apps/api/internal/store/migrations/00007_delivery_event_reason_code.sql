-- +goose Up
-- The R-02 failure reason chip. Required on a FAILED outcome by the delivery
-- service; nullable so existing rows and DELIVERED/DELAYED events stay valid.
-- Vocabulary mirrors DELIVERY_FAILURE_REASONS in libs/shared-types.
ALTER TABLE delivery_event
    ADD COLUMN IF NOT EXISTS reason_code TEXT
    CHECK (reason_code IN ('OUTLET_CLOSED', 'ACCESS_BLOCKED', 'REFUSED_BY_STORE', 'GOODS_DAMAGED', 'OTHER'));

-- +goose Down
ALTER TABLE delivery_event DROP COLUMN IF EXISTS reason_code;
