-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Immutable wrapper so unaccent can be used in index expressions / tsvector.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION immutable_unaccent(text) RETURNS text AS $$
SELECT public.unaccent('public.unaccent', $1)
$$ LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS immutable_unaccent(text);
DROP FUNCTION IF EXISTS set_updated_at();
DROP EXTENSION IF EXISTS unaccent;
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pgcrypto;
