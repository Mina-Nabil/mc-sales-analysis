-- Saved analytics views: named dashboard snapshots (chart set + filters + year).
-- Shared across all users. `config` is opaque JSON owned by the frontend
-- (bars, pies, filters, year) so the shape can evolve without a migration.
CREATE TABLE saved_views (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       TEXT NOT NULL,
  config     JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON saved_views (created_at);
