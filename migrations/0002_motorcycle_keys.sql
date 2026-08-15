-- Motorcycle recognition for the LIVE monthly feed (§0.1, §11.6).
-- The monthly primary feed has no derived Car Type column, so the §0.1 rule
-- (which keys off car_type/segment/engine) cannot fire at parse time. This table
-- carries the normalized raw keys that history proved to be motorcycles, so the
-- importer can drop + count them when ingest.exclude_motorcycles = true.
--
-- Keys are the tier-2b no-space normalized form (domain.NormalizeNoSpace).
--   kind='brand'        → any row whose brand_key matches is a motorcycle
--   kind='brand_model'  → row matches only if BOTH brand_key and model_key match
-- Generated from the historical source workbook by Phase 0 recognition.

CREATE TABLE motorcycle_keys (
  id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  kind      TEXT NOT NULL,          -- 'brand' | 'brand_model'
  brand_key TEXT NOT NULL,
  model_key TEXT NOT NULL DEFAULT '',
  UNIQUE (kind, brand_key, model_key)
);
CREATE INDEX ON motorcycle_keys (brand_key);
