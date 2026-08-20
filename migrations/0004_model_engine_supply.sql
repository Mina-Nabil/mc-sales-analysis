-- Model-level engine_type / supply DEFAULTS (§2.8 refinement).
--
-- engine_type and supply are fact-level: the historical workbook carried them
-- per row, so per-period nuance (a model that switched ICE→HYBRID, or SUP→CKD)
-- is preserved on facts. But the live monthly primary feed has NO engine/supply
-- columns, so facts imported from it are NULL — which made the engine/supply
-- filters return nothing for those months.
--
-- These columns hold the model's *typical* engine/supply (its highest-volume
-- historical value). Analytics use COALESCE(fact, model) so a fact with no
-- source engine falls back to the model's known one — and editing the model
-- re-derives every such fact (§2.5). Facts keep their own value when present.
ALTER TABLE models ADD COLUMN engine_type TEXT;
ALTER TABLE models ADD COLUMN supply TEXT;
