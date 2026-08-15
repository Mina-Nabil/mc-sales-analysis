-- Store a one-line human-readable reason on a proposed alias (§3.4 output
-- "reasoning", shown to the reviewer). Applies to fuzzy and, later, AI proposals.
ALTER TABLE model_aliases ADD COLUMN reasoning TEXT;
ALTER TABLE brand_aliases ADD COLUMN reasoning TEXT;
