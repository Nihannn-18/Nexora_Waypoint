-- +goose Up
-- Go-owned authentication.
--
-- Authentication is owned by the Go API: app_user now carries the credential
-- (a password hash), and a session table stores opaque session tokens by their
-- hash. There is no separate identity system and no cross-system mapping:
-- session.user_id references app_user.user_id directly.

-- Credentials. Existing rows get an empty hash and therefore cannot log in
-- until the seed (or an operator) sets one; login rejects an empty hash.
ALTER TABLE app_user ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';

-- Display name for the signed-in user. The supplied CSVs carry no names, so the
-- seed sets the four demo personas; it is not an authorization field.
ALTER TABLE app_user ADD COLUMN display_name TEXT NOT NULL DEFAULT '';

-- Opaque server-side sessions. Only the SHA-256 hash of the raw token is
-- stored, so a database read cannot yield a usable credential. The raw token is
-- returned to the client exactly once, at login.
CREATE TABLE session (
    session_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    TEXT NOT NULL REFERENCES app_user (user_id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX session_user_idx ON session (user_id);
CREATE INDEX session_expires_idx ON session (expires_at);

-- +goose Down
DROP TABLE IF EXISTS session;
ALTER TABLE app_user DROP COLUMN IF EXISTS display_name;
ALTER TABLE app_user DROP COLUMN IF EXISTS password_hash;
