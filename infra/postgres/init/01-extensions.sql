-- Runs once, on an empty data volume only, before the API first connects.
--
-- Scope: extensions and anything the schema depends on existing beforehand.
-- Tables, indexes and seed rows do NOT belong here — they belong in the API's
-- migration step, so they are versioned and replayable. This file is ignored on
-- every start after the first; `docker compose down -v` forces a re-run.

-- gen_random_uuid() for primary keys.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Trigram indexes, for outlet name search on the dispatcher's order queue.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- The business day is Asia/Colombo. Setting it at the database level means a
-- bare `now()` in a query or a migration cannot silently produce a UTC date and
-- shift an order onto the wrong delivery day.
ALTER DATABASE waypoint SET timezone TO 'Asia/Colombo';
