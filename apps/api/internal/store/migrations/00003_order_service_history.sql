-- +goose Up
-- Deferral history the prioritisation policy reads: whether the order's outlet
-- was deferred on the previous run, and how many days it has gone unserved.
-- Seeded from the Task 2B scenario; NULL means no history recorded.
ALTER TABLE customer_order
    ADD COLUMN deferred_yesterday      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN days_since_last_served  INTEGER CHECK (days_since_last_served >= 0);
