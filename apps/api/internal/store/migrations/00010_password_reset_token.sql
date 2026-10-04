-- +goose Up
-- Account-management support: a creation timestamp and self-service reset
-- tokens.
--
-- app_user previously carried no creation time, so the Dispatcher user screen
-- (a required deliverable) could not show a "created" date. Adding one column
-- is the smallest change that supports it; existing rows get now() at migration
-- time, which is accurate enough for the seeded accounts.
ALTER TABLE app_user ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Self-service password reset tokens.
--
-- A Dispatcher creates operational accounts and sets an initial password, but
-- no human should know another user's password afterwards. When a user forgets
-- theirs, the reset flow mints a short-lived, single-use token. Only the
-- SHA-256 hash of the raw token is stored — exactly as opaque sessions do — so
-- a database read cannot reconstruct a usable reset link.
--
-- The flow is:
--   * request: mint a random token, store its hash, return a generic success
--     regardless of whether the email exists (no account enumeration);
--   * reset: look the token hash up, require not-used and not-expired, set the
--     new Argon2id password hash, mark the token used, and revoke the user's
--     existing sessions so the old credential cannot be replayed.
--
-- Multiple outstanding tokens per user are tolerated; a new request is a new
-- row, and every prior unused row for that user is invalidated in the same
-- transaction so only the most recent link works. used_at makes a token
-- single-use even if it has not yet expired.
CREATE TABLE password_reset_token (
    token_id   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT NOT NULL REFERENCES app_user (user_id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_token_user_idx ON password_reset_token (user_id);
CREATE INDEX password_reset_token_expires_idx ON password_reset_token (expires_at);

-- +goose Down
DROP TABLE IF EXISTS password_reset_token;
ALTER TABLE app_user DROP COLUMN IF EXISTS created_at;

