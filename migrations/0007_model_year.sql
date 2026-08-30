-- Model year as a fact dimension.
--
-- facts.model_year already exists (0001, under "optional enrichment") but was
-- never written or read. The live monthly feed moves to the traffic authority's
-- "brands & models of zero vehicles by year of manufacture" report, which is an
-- exact decomposition of the by-status feed's Zero column at a finer grain, so
-- model_year now carries real data from 2026 onward.
--
-- model_year is observed source data, not derived from the tree, so storing it
-- on the fact does not violate §2.5 ("nothing derived is stored on a fact").
-- It is NULL for Feb-2021 → Dec-2025: the source workbook has no manufacture-year
-- column and no raw archive survives for those months, so 'Unknown' is the
-- honest value rather than an inferred one.

-- Sanity bound. NULL stays valid — it is the permanent state for pre-2026 facts.
ALTER TABLE facts
  ADD CONSTRAINT facts_model_year_range
  CHECK (model_year IS NULL OR model_year BETWEEN 1980 AND 2100);

-- The natural key gains model_year: one source row now unpivots into one fact
-- per model year, so (gov, unit, brand, model, period) alone is no longer the
-- grain. Still non-unique — the 2021–2024 history has 808 duplicate groups that
-- a faithful migration must keep (§8.1).
CREATE INDEX facts_natural_key_idx ON facts
  (raw_governorate, raw_unit, raw_brand, raw_model,
   period_year, period_month, model_year, import_batch_id);

DROP INDEX IF EXISTS facts_raw_governorate_raw_unit_raw_brand_raw_model_period_y_idx;

-- Scan support for the model_year / model_age dimensions.
CREATE INDEX facts_period_year_model_year_idx ON facts (period_year, model_year);
