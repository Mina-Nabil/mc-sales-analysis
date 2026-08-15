-- Egypt Car Sales Analytics — initial schema
-- Implements the data model in TECHNICAL_REQUIREMENTS.md §2.
-- Design rule (§2.5): nothing derived is stored on the fact. Segment, origin,
-- distributor, region are all resolved by JOIN to the trees, so fixing one node
-- re-derives all history.

CREATE EXTENSION IF NOT EXISTS btree_gist;   -- required by distributor_assignments EXCLUDE

CREATE TYPE resolution_status AS ENUM (
  'confirmed', 'auto_resolved', 'needs_review', 'unresolved', 'rejected'
);

-- ── Users & auth (§11.5) ───────────────────────────────────────────────────
CREATE TABLE users (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,               -- opaque random token
  user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX ON sessions (user_id);

-- ── Car tree (§2.1) ────────────────────────────────────────────────────────
CREATE TABLE segments (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,           -- body type: Sedan, Compact SUV, ...
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE brands (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name            TEXT NOT NULL UNIQUE,       -- canonical English, e.g. 'MG'
  parent_brand_id BIGINT REFERENCES brands(id),
  origin          TEXT,                       -- China | Europe | Japan | Korea | ...
  status          resolution_status NOT NULL DEFAULT 'confirmed',
  notes           TEXT,
  merged_into     BIGINT REFERENCES brands(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON brands (parent_brand_id);

CREATE TABLE models (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  brand_id    BIGINT NOT NULL REFERENCES brands(id),
  name        TEXT NOT NULL,                  -- 'MG5'
  car_type    TEXT,                           -- Passenger|Commercial|Bus|Construction
  segment_id  BIGINT REFERENCES segments(id),
  tier        TEXT,                           -- Highline | Baseline (model-level)
  -- NOTE: engine_type and supply are NOT here — they are fact-level (§2.8).
  status      resolution_status NOT NULL DEFAULT 'confirmed',
  merged_into BIGINT REFERENCES models(id),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (brand_id, name)
);
CREATE INDEX ON models (segment_id);

-- ── Aliases (§2.2) — the central mechanic ──────────────────────────────────
CREATE TABLE brand_aliases (
  id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  raw                    TEXT NOT NULL,        -- exactly as it appeared
  raw_normalized         TEXT NOT NULL,        -- §3.2 normalized key
  raw_normalized_nospace TEXT NOT NULL,        -- §3.2 rule 11 (tier 2b)
  brand_id               BIGINT REFERENCES brands(id),
  status                 resolution_status NOT NULL,
  confidence             REAL,
  method                 TEXT NOT NULL,        -- exact|normalized|fuzzy|ai|human|seed
  decided_by             BIGINT REFERENCES users(id),
  decided_at             TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (raw)
);
CREATE INDEX ON brand_aliases (raw_normalized);
CREATE INDEX ON brand_aliases (raw_normalized_nospace);

CREATE TABLE model_aliases (
  id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  brand_id               BIGINT NOT NULL REFERENCES brands(id),   -- aliases are per-brand
  raw                    TEXT NOT NULL,
  raw_normalized         TEXT NOT NULL,
  raw_normalized_nospace TEXT NOT NULL,
  model_id               BIGINT REFERENCES models(id),
  status                 resolution_status NOT NULL,
  confidence             REAL,
  method                 TEXT NOT NULL,
  decided_by             BIGINT REFERENCES users(id),
  decided_at             TIMESTAMPTZ,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (brand_id, raw)
);
CREATE INDEX ON model_aliases (brand_id, raw_normalized);
CREATE INDEX ON model_aliases (brand_id, raw_normalized_nospace);

-- ── Geography (§2.3) — no coordinates, no map ──────────────────────────────
CREATE TABLE regions (
  id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);
CREATE TABLE governorates (
  id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name      TEXT NOT NULL UNIQUE,
  region_id BIGINT NOT NULL REFERENCES regions(id)
);
CREATE TABLE traffic_units (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name           TEXT NOT NULL UNIQUE,
  governorate_id BIGINT NOT NULL REFERENCES governorates(id)
);
CREATE TABLE geo_aliases (
  id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  raw                    TEXT NOT NULL,
  raw_normalized         TEXT NOT NULL,
  raw_normalized_nospace TEXT NOT NULL,
  kind                   TEXT NOT NULL,        -- 'governorate' | 'traffic_unit'
  target_id              BIGINT,
  status                 resolution_status NOT NULL,
  confidence             REAL,
  method                 TEXT NOT NULL,
  UNIQUE (kind, raw)
);
CREATE INDEX ON geo_aliases (raw_normalized);

-- ── Distributors (§2.4) — effective-dated, optional ────────────────────────
CREATE TABLE distributors (
  id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);
CREATE TABLE distributor_assignments (
  id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  brand_id       BIGINT NOT NULL REFERENCES brands(id),
  car_type       TEXT NOT NULL,
  distributor_id BIGINT NOT NULL REFERENCES distributors(id),
  valid_from     DATE NOT NULL,
  valid_to       DATE,
  EXCLUDE USING gist (
    brand_id WITH =, car_type WITH =,
    daterange(valid_from, COALESCE(valid_to, 'infinity'::date), '[)') WITH &&
  )
);
CREATE INDEX ON distributor_assignments (brand_id, car_type);

-- ── Imports & audit (§2.6) ─────────────────────────────────────────────────
CREATE TABLE import_batches (
  id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  source_filename     TEXT,
  file_s3_key         TEXT,
  file_sha256         TEXT,
  feed_role           TEXT,                    -- primary | enrichment | reconciliation
  detected_grain      TEXT,
  period_year         SMALLINT,
  period_month        SMALLINT,
  state               TEXT NOT NULL,           -- uploaded|parsed|dry_run|committed|rolled_back|failed
  row_count           INTEGER,
  total_volume        BIGINT,
  dropped_row_count   INTEGER,                 -- motorcycles etc. (§0.1)
  dropped_volume      BIGINT,
  supersedes_batch_id BIGINT REFERENCES import_batches(id),
  revision_reason     TEXT,
  created_by          BIGINT REFERENCES users(id),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  committed_at        TIMESTAMPTZ
);

-- ── Facts (§2.5) ───────────────────────────────────────────────────────────
CREATE TABLE facts (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  -- raw identity — permanently immutable
  raw_governorate TEXT NOT NULL,
  raw_unit        TEXT NOT NULL,
  raw_brand       TEXT NOT NULL,
  raw_model       TEXT NOT NULL,
  period_year     SMALLINT NOT NULL,
  period_month    SMALLINT NOT NULL,
  volume          INTEGER NOT NULL,
  -- resolved references (nullable = unresolved)
  brand_id        BIGINT REFERENCES brands(id),
  model_id        BIGINT REFERENCES models(id),
  governorate_id  BIGINT REFERENCES governorates(id),
  traffic_unit_id BIGINT REFERENCES traffic_units(id),
  -- provenance
  import_batch_id BIGINT NOT NULL REFERENCES import_batches(id),
  status          resolution_status NOT NULL,
  -- fact-level dimensions (§2.8) + optional enrichment
  engine_type     TEXT,      -- ICE|BEV|HYBRID|REEV|Other
  supply          TEXT,      -- CKD | SUP
  license_type    TEXT,
  vehicle_status  TEXT,
  model_year      SMALLINT,
  body_shape      TEXT,
  engine_cc       INTEGER,
  fuel_type       TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
  -- NOTE: TECH §2.5 specifies a UNIQUE natural key per batch. The natural key
  -- is proven unique from 2026 onward (17,600/17,600), but the historical
  -- source contains 808 genuinely-duplicate (gov,unit,brand,model,period)
  -- groups (5,061 rows) in 2021–2024 that differ in no raw or fact-level
  -- column. Faithful migration must retain all 377,425 rows (§8.1), so the key
  -- is a non-unique index here; uniqueness is asserted by query where it holds.
);
CREATE INDEX ON facts (raw_governorate, raw_unit, raw_brand, raw_model,
                       period_year, period_month, import_batch_id);
CREATE INDEX ON facts (period_year, period_month);
CREATE INDEX ON facts (brand_id, period_year, period_month);
CREATE INDEX ON facts (model_id, period_year, period_month);
CREATE INDEX ON facts (status) WHERE status IN ('needs_review', 'unresolved');

-- ── Change log (§2.6) — the sole control; written for EVERY mutation ────────
CREATE TABLE change_log (
  id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  entity_type   TEXT NOT NULL,
  entity_id     BIGINT,
  action        TEXT NOT NULL,
  before        JSONB,
  after         JSONB,
  volume_impact BIGINT,
  actor_id      BIGINT REFERENCES users(id),
  actor_kind    TEXT NOT NULL,               -- human | agent | migration
  batch_id      BIGINT REFERENCES import_batches(id),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON change_log (entity_type, entity_id, created_at DESC);

-- ── Settings (§3.5) ────────────────────────────────────────────────────────
CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings (key, value) VALUES
  ('fuzzy_threshold',    '0.92'),
  ('fuzzy_review_floor', '0.75'),
  ('ai_threshold',       '0.90'),
  ('ai_model',           'eu.anthropic.claude-sonnet-4-5-20250929-v1:0'),
  ('ingest.exclude_motorcycles', 'true');
