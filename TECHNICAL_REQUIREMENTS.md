# Egypt Car Sales Analytics — Technical Requirements

**Build specification for implementing agents.**
**Stack: Go (backend) · React (frontend) · PostgreSQL · AWS**
**Companions:** `BUSINESS_LOGIC_PLAN.md` (why), `WEB_APP_PROPOSAL.md` (scope & cost)

> Read `BUSINESS_LOGIC_PLAN.md` first. This document assumes its domain model and does not restate the reasoning.

---

## 0. Ground truth — measured, not assumed

Every figure below was measured from `260308_Registration Report Dashboard Inc Distributor.xlsx`, **after applying the two scope rules in §0.1**. Use them for sizing and for acceptance tests.

| Fact | Value |
|---|---|
| Fact rows (existing history) | **377,425** |
| Total units | **1,107,814** |
| Period covered | Feb-2021 → Feb-2026 — 60 periods present; Jan-2021 and May-2021 absent from the source |
| New rows per month | ~8,000 (last 12 months avg: 8,031) |
| Distinct raw Arabic brand strings | 781 |
| Distinct raw (brand, model) pairs | 3,779 |
| Distinct raw governorate strings | 29 |
| Distinct raw traffic-unit strings | 222 |
| Canonical brands / models / segments in use | **201 / 1,243 / 13** (24-value segment vocabulary) |
| **New (brand, model) pairs per month** | **~64** (last 12 months avg) |
| New brands per month | ~7 |
| Fact grain | `(governorate_ar, unit_ar, brand_ar, model_ar, month, year)` — **verified unique**, 17,600/17,600 for 2026 |

**Implication for design:** this is a small dataset. Do not introduce a data warehouse, OLAP engine, message broker, or Kubernetes. PostgreSQL with correct indexes answers every analytical query in this spec in milliseconds. Complexity here is a defect.

### 0.1 Scope rules — apply at ingestion

Two rules narrow the dataset. Both are business decisions, both are implemented in the importer, and both are already reflected in the Phase 0 seed files.

**Rule 1 — Motorcycles are out of scope. Drop them at ingestion.**

A row is a motorcycle if **any** of: `car_type = 'Motorcycle'` · `segment ∈ ('Motorcycle','Scooter')` · `engine = 'Motorcycle'` · `brand = 'Motorcycle'`. Measured on history these four agree on all but 215 units, so the OR is safe.

This removes **148,921 rows / 1,138,473 units — 50.7% of the raw file.**

> **Do not silently discard.** The import dry-run must report dropped rows and dropped volume as an explicit line, so that a month's ingested total plus dropped total still reconciles against the source file. Reconciliation against authority PDFs (§4.5) compares against the *pre-filter* total.

**Rule 2 — Brand AND model names are case-consolidated.**

Applied in this order, to brands and models alike:

| # | Rule | Count |
|---|---|---:|
| 1 | Name is **≤4 letters** → **ALL CAPS**, always | 53 brands |
| 2 | Both spellings exist and rule 1 does not apply → merge into the normal-case form | 14 |
| 3 | All-caps only, **≥5 letters** (`VIGOREY`) → treat as a word, Title case (`Vigorey`) | 18 |

**Rule 1 is absolute and deterministic** — `KIA`, `SEAT`, `FIAT`, `AUDI`, `JEEP`, `FORD`, `OPEL`, `BAIC`, `JAC`, `LADA`, `MAN`, `TATA`, `FAW`, `NIO`, `AITO`, `NETA` alongside `MG`, `BMW`, `BYD`, `DS`, `GMC`. No per-brand judgement, so the importer applies it to unseen brands forever without a lookup table. Note it diverges from a few manufacturers' own styling (Audi, Jeep, Ford, Opel style themselves in title case) — that is accepted in exchange for a rule with no exceptions.

The identical rule is applied to **model** names — `Tiggo4 PRO`→`Tiggo4 Pro`, `GLORY`→`Glory`, `TAYRON`→`Tayron`, `I3`→`i3` (8 groups, 11,136 units).

Going forward the importer applies the same rule to newly-seen brands and models, and anything it re-cases or merges automatically is logged to `change_log`.

---

## 1. Architecture

```
                      ┌──────────────────────────────────────┐
  Cloudflare Tunnel   │  ECS Fargate — ONE task              │
  (approved, no ALB)  │  0.5 vCPU / 1 GB · ARM64             │
┌─────────────┐       │                                      │
│ React SPA   │──────▶│  Go binary:                          │
│ served by   │       │   • REST + JSON API                  │
│ the API     │◀──────│   • SSE for job progress             │
│ (embedded)  │       │   • embedded React build (embed.FS)  │
└─────────────┘       │   • import worker as a GOROUTINE     │
                      └──────────────┬───────────────────────┘
                                     ├──▶ PostgreSQL — RDS db.t4g.small
                                     ├──▶ S3 (uploads, exports, tree dumps)
                                     └──▶ AWS Bedrock — Claude (IAM role, VPC endpoint)

NO load balancer.  NO NAT gateway.  NO CloudFront.  NO second task.
```

**Deliberate choices:**

- **One Go binary, one running task.** The API serves the React build from `embed.FS`, and the import worker runs as a goroutine inside the same process. Retain a `--worker` flag that runs *only* the worker loop, so the job can be split onto its own task later without a code change — but **do not deploy it that way now**. At a few imports a month, a second 24/7 container costs $18/month to sit idle.
- **Job queue in Postgres** (`SELECT … FOR UPDATE SKIP LOCKED`). At a few imports a month, SQS/Redis is unjustified.
- **No ORM for analytics.** Use `sqlc` or hand-written SQL for the query layer. Analytical queries are the product; they should be readable SQL, not generated joins.
- **Server-side aggregation always.** Never ship fact rows to the browser for it to sum.

---

## 2. Data model

PostgreSQL. All tables have `id BIGINT GENERATED ALWAYS AS IDENTITY`, `created_at`, `updated_at` unless noted.

### 2.1 Car tree

```sql
CREATE TABLE brands (
  id            BIGINT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,          -- canonical English, e.g. 'MG'
  parent_brand_id BIGINT REFERENCES brands(id),-- sub-brand link; see §2.9
  origin        TEXT,                          -- China | Europe | Japan | Korea | USA
                                               -- | India | Russia | UAE | Egypt | Others
  status        resolution_status NOT NULL DEFAULT 'confirmed',
  notes         TEXT,
  merged_into   BIGINT REFERENCES brands(id)   -- non-null = redirect after merge
);

CREATE TABLE models (
  id            BIGINT PRIMARY KEY,
  brand_id      BIGINT NOT NULL REFERENCES brands(id),
  name          TEXT NOT NULL,                 -- 'MG5'
  car_type      TEXT,                          -- Passenger|Commercial|Bus|Construction
  segment_id    BIGINT REFERENCES segments(id),
  tier          TEXT,                          -- Highline | Baseline   (model-level)
  -- NOTE: engine_type and supply (CKD/SUP) are NOT here. They are fact-level; see §2.8.
  status        resolution_status NOT NULL DEFAULT 'confirmed',
  merged_into   BIGINT REFERENCES models(id),
  UNIQUE (brand_id, name)
);

CREATE TABLE segments (
  id   BIGINT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE
);
-- Segments are BODY TYPES, not size codes. No size_class, no 'Luxury ' prefix —
-- size is dropped entirely and premium-ness lives in models.tier.
-- Controlled vocabulary (24), seeded by Phase 0:
--   Sedan · Hatchback · Crossover · Compact SUV · SUV · Coupe · Convertible
--   Roadster · Station Wagon · Liftback · MPV · Minivan · Van · Pickup
--   Minibus · Bus · Microcar · Limousine · Sports car · Supercar · Hypercar
--   Off-roader · Truck · Construction
-- 13 are populated from history; the remainder are available for users and the
-- agent to assign to new models. See §2.7.
```

`resolution_status` is an enum: `confirmed | auto_resolved | needs_review | unresolved | rejected`.

### 2.2 Aliases — the central mechanic

```sql
CREATE TABLE brand_aliases (
  id             BIGINT PRIMARY KEY,
  raw            TEXT NOT NULL,           -- exactly as it appeared, e.g. 'ام جـى'
  raw_normalized TEXT NOT NULL,           -- see §3.2
  brand_id       BIGINT REFERENCES brands(id),
  status         resolution_status NOT NULL,
  confidence     REAL,                    -- 0..1, null for human decisions
  method         TEXT NOT NULL,           -- exact|normalized|fuzzy|ai|human
  decided_by     BIGINT REFERENCES users(id),
  decided_at     TIMESTAMPTZ,
  UNIQUE (raw)
);

CREATE TABLE model_aliases (
  id             BIGINT PRIMARY KEY,
  brand_id       BIGINT NOT NULL REFERENCES brands(id),  -- scope: aliases are per-brand
  raw            TEXT NOT NULL,
  raw_normalized TEXT NOT NULL,
  model_id       BIGINT REFERENCES models(id),
  status         resolution_status NOT NULL,
  confidence     REAL,
  method         TEXT NOT NULL,
  decided_by     BIGINT REFERENCES users(id),
  decided_at     TIMESTAMPTZ,
  UNIQUE (brand_id, raw)
);
CREATE INDEX ON model_aliases (brand_id, raw_normalized);
```

**Model aliases are scoped to a brand.** `Tiggo 7` under Chery and under Jetour are different cars. Never resolve a model alias globally.

### 2.3 Geography

```sql
CREATE TABLE regions      (id, name UNIQUE);                    -- 8
CREATE TABLE governorates (id, name UNIQUE, region_id FK);      -- 27
CREATE TABLE traffic_units(id, name UNIQUE, governorate_id FK);  -- 210
-- NO coordinates. There is no map view in scope; storing lat/long would be dead weight.
CREATE TABLE geo_aliases  (id, raw, raw_normalized, kind, target_id,
                           status, confidence, method, ...);
-- kind ∈ ('governorate','traffic_unit')
```

⚠️ **Migration warnings — both confirmed in Phase 0.**

1. In the source workbook's `Name` sheet, columns `A/B` (Arabic→English unit), `G/H/I/J` (Region/Area/coords) and `N/O` (Brand→Origin) are **three unrelated lists that merely share rows**. Row 22 places `Assiut` beside `Canal`, which is meaningless. Do not import that sheet as a joined table — Phase 0 rebuilt the hierarchy from co-occurrence in the fact data instead.

2. *(No longer relevant — coordinates are out of scope. For the record: the sheet's `Longtiude` and `Latitude` columns hold reversed values, so anyone who later adds a map must not trust those headers.)*

3. **Ignore the coordinate columns entirely.** A map view is explicitly out of scope, so the swapped/missing coordinates are moot — do not import them.

### 2.4 Distributors — effective-dated

```sql
CREATE TABLE distributors (id, name UNIQUE);   -- 39

CREATE TABLE distributor_assignments (
  id             BIGINT PRIMARY KEY,
  brand_id       BIGINT NOT NULL REFERENCES brands(id),
  car_type       TEXT NOT NULL,          -- assignment keys on (brand, car_type)
  distributor_id BIGINT NOT NULL REFERENCES distributors(id),
  valid_from     DATE NOT NULL,          -- seed: 2021-01-01
  valid_to       DATE,                   -- null = current
  EXCLUDE USING gist (
    brand_id WITH =, car_type WITH =,
    daterange(valid_from, COALESCE(valid_to,'infinity'::date), '[)') WITH &&
  )
);
```

The `EXCLUDE` constraint makes overlapping assignments impossible at the database level. A fact resolves its distributor by the assignment covering **its own month**, not today's.

**Distributor is optional and stays optional.** Many brands have no distributor and never will. Model this as the *absence of a row* in `distributor_assignments` — never as a sentinel string like `'Undefined'`. Consequences the build must honour:

- A fact whose brand has no assignment resolves `distributor_id → NULL`. This is a valid, permanent state, **not** an unresolved-data condition, and it must never enter the review queue.
- Reports group NULL under a visible **"No distributor"** bucket rather than hiding those rows.
- Users can set, change or clear an assignment at any time from the tree settings screen, with the effective dates they choose.
- Phase 0 seeds **138 real assignments**; 130 brand/car-type combinations (21,394 units) are deliberately left with none.

### 2.5 Facts

```sql
CREATE TABLE facts (
  id              BIGINT PRIMARY KEY,
  -- raw identity — permanently immutable
  raw_governorate TEXT NOT NULL,
  raw_unit        TEXT NOT NULL,
  raw_brand       TEXT NOT NULL,
  raw_model       TEXT NOT NULL,
  period_year     SMALLINT NOT NULL,
  period_month    SMALLINT NOT NULL,     -- 1..12
  volume          INTEGER NOT NULL,
  -- resolved references (nullable = unresolved)
  brand_id        BIGINT REFERENCES brands(id),
  model_id        BIGINT REFERENCES models(id),
  governorate_id  BIGINT REFERENCES governorates(id),
  traffic_unit_id BIGINT REFERENCES traffic_units(id),
  -- provenance
  import_batch_id BIGINT NOT NULL REFERENCES import_batches(id),
  status          resolution_status NOT NULL,
  -- optional enrichment dimensions (null unless an enrichment feed supplied them)
  engine_type     TEXT,      -- ICE|BEV|HYBRID|REEV|Other   <- fact-level, see §2.8
  supply          TEXT,      -- CKD | SUP                    <- fact-level, see §2.8
  license_type    TEXT,
  vehicle_status  TEXT,      -- Used | Zero | جمارك | هيئه دبلوماسية
  model_year      SMALLINT,
  body_shape      TEXT,
  engine_cc       INTEGER,
  fuel_type       TEXT,
  UNIQUE (raw_governorate, raw_unit, raw_brand, raw_model,
          period_year, period_month, import_batch_id)
);

CREATE INDEX ON facts (period_year, period_month);
CREATE INDEX ON facts (brand_id, period_year, period_month);
CREATE INDEX ON facts (model_id, period_year, period_month);
CREATE INDEX ON facts (status) WHERE status IN ('needs_review','unresolved');
```

**Nothing derived is stored on the fact.** No segment, no origin, no distributor, no region — those come from joins to the trees. This is what makes "fix one node, correct all history" work, and it is non-negotiable.

### 2.7 Segment vocabulary — body type, research-derived

Segments are **body types**. Size codes are gone; premium-ness lives in `models.tier`.

| Historical | → | Segment |
|---|---|---|
| `Car-A` … `Car-F`, `Car-1` … `Car-5` | → | **Sedan** |
| `Car HB-A/B/C`, `HB` | → | **Hatchback** |
| `MPV-*` · `Van-*` · `Pickup-*` · `Truck-*` | → | **MPV · Van · Pickup · Truck** |
| `Mid-Bus` · `Large Bus` · `Sport` · `Construction` | → | **Minibus · Bus · Sports car · Construction** |

⚠️ **The SUV family was NOT mapped from the size codes — the codes are unreliable.** The source coded 7-seat midsize vehicles (Skoda Kodiaq, Chery Tiggo 8, Chevrolet Captiva) and subcompacts (Kia Seltos, Suzuki Vitara, Toyota Urban Cruiser) all as `SUV-C`. Phase 0 instead **researched the 150 models comprising 97% of SUV volume** — length, wheelbase and seat rows — and classified each directly.

**The rule that was applied, and that the agent must apply to new models:**

| Bucket | Criteria |
|---|---|
| **Crossover** | A/B-segment. Under ~4,400 mm, **or** built on a B-segment platform (wheelbase < 2,650 mm) |
| **Compact SUV** | C-segment. ~4,400–4,700 mm, 2 rows, C-segment platform |
| **SUV** | D/E/F-segment. Over ~4,700 mm, **or** any 3-row / 7-seat model regardless of length |

Two tie-breakers that matter, both learned from real conflicts in the research:

1. **Platform beats raw length.** Škoda Karoq is 4,390 mm but sits on the C-segment MQB platform → Compact SUV. Škoda Kamiq is 4,241 mm on MQB-A0 → Crossover.
2. **Seat rows beat length.** Toyota Rush is only 4,435 mm but is sold in Egypt as a 7-seater → SUV.
3. **No luxury exception.** Premium brands use the same buckets by size. Mercedes GLA → Crossover, GLC (4,749 mm) → SUV. BMW X1 → Compact SUV, X3 (4,755 mm) → SUV.

**Result: 50 models reclassified, 98,619 units — 23% of SUV volume.** Five entries were not SUVs at all: Peugeot 408 and Citroën C4X are fastback **sedans**; Citroën C4, DS 4 and Zeekr 001 are **hatchbacks**.

`phase0/seed/segment_research.csv` carries all 150 researched classifications with notes, and `models.csv` has a `segment_source` column distinguishing `researched` from `inferred from old size code`. **Load the researched values; do not re-derive them from the size codes.**

The 344 remaining SUV-family models (3% of SUV volume) still carry an inferred segment, are marked `needs_review`, and appear as a single DECISION item in the queue rather than 344 separate ones.

### 2.8 `supply` and `engine_type` belong to the FACT, not the model

This was measured, not assumed. Both attributes change over a model's life, so storing them on the model node manufactures conflicts that are not conflicts.

**Evidence — Hyundai Elantra AD, CKD vs SUP by year:**

| 2021 | 2022 | 2023 | 2024 | 2025 | 2026 |
|---|---|---|---|---|---|
| SUP 30 | SUP 17 | SUP 2 | **CKD 3,946** | **CKD 9,023** | **CKD 1,500** |

That is not a data error — GB Auto began local assembly of the Elantra AD in **May 2024** (independently confirmed by research). The same clean transition appears in Proton Saga, Geely Coolray, Citroën C4X, Jetour X70 Plus and Foton View; Kia Sorento went the other way, from CKD to imported.

Across all 50 conflicted (model, attribute) pairs, **67% of the affected volume is explained by a clean year-by-year transition.** The remainder are ramp-up years where both an imported and a locally-assembled batch legitimately shipped in the same period.

**Consequence for the build:** carry `supply` and `engine_type` on `facts`, populated per import from the source row. They are reportable dimensions and may be filtered like any other, but they are never model attributes and must never enter the review queue as conflicts. This alone removed **46 items** from the Phase 0 queue.

### 2.9 Sub-brands are separate brands with a parent link

Research confirmed each of these is a distinct marque in Egypt with its own badge and price list, and several have a **different distributor from their parent** — Omoda, IM Motors, Zeekr and Deepal all do. Merging them would destroy exactly the dimension the business reports on.

| Sub-brand | Parent | | Sub-brand | Parent |
|---|---|---|---|---|
| Jetour, Exeed, Omoda | Chery | | DS | Citroën |
| Denza, Yangwang | BYD | | Cupra | SEAT |
| IM Motors | MG | | Zeekr | Geely |
| Deepal | Changan | | Lexus | Toyota |

`parent_brand_id` gives "Chery Group = Chery + Jetour + Exeed + Omoda" roll-ups without hard-coding. Two edge cases the build must handle:

- **Renames are a merge, not a parent link.** SsangYong → KGM is one brand record with an alias and a rename date, so 2021 SsangYong and 2026 KGM sales trend as a single line.
- **Badging is market-specific.** IM is a standalone brand in Egypt but sold as "MG IM" in Europe. Resolve brand→parent per market, not globally.

### 2.6 Imports and audit### 2.8 `supply` and `engine_type` belong to the FACT, not the model

This was measured, not assumed. Both attributes change over a model's life, so storing them on the model node manufactures conflicts that are not conflicts.

**Evidence — Hyundai Elantra AD, CKD vs SUP by year:**

| 2021 | 2022 | 2023 | 2024 | 2025 | 2026 |
|---|---|---|---|---|---|
| SUP 30 | SUP 17 | SUP 2 | **CKD 3,946** | **CKD 9,023** | **CKD 1,500** |

That is not a data error — GB Auto began local assembly of the Elantra AD in **May 2024** (independently confirmed by research). The same clean transition appears in Proton Saga, Geely Coolray, Citroën C4X, Jetour X70 Plus and Foton View; Kia Sorento went the other way, from CKD to imported.

Across all 50 conflicted (model, attribute) pairs, **67% of the affected volume is explained by a clean year-by-year transition.** The remainder are ramp-up years where both an imported and a locally-assembled batch legitimately shipped in the same period.

**Consequence for the build:** carry `supply` and `engine_type` on `facts`, populated per import from the source row. They are reportable dimensions and may be filtered like any other, but they are never model attributes and must never enter the review queue as conflicts. This alone removed **46 items** from the Phase 0 queue.

### 2.9 Sub-brands are separate brands with a parent link

Research confirmed each of these is a distinct marque in Egypt with its own badge and price list, and several have a **different distributor from their parent** — Omoda, IM Motors, Zeekr and Deepal all do. Merging them would destroy exactly the dimension the business reports on.

| Sub-brand | Parent | | Sub-brand | Parent |
|---|---|---|---|---|
| Jetour, Exeed, Omoda | Chery | | DS | Citroën |
| Denza, Yangwang | BYD | | Cupra | SEAT |
| IM Motors | MG | | Zeekr | Geely |
| Deepal | Changan | | Lexus | Toyota |

`parent_brand_id` gives "Chery Group = Chery + Jetour + Exeed + Omoda" roll-ups without hard-coding. Two edge cases the build must handle:

- **Renames are a merge, not a parent link.** SsangYong → KGM is one brand record with an alias and a rename date, so 2021 SsangYong and 2026 KGM sales trend as a single line.
- **Badging is market-specific.** IM is a standalone brand in Egypt but sold as "MG IM" in Europe. Resolve brand→parent per market, not globally.

### 2.6 Imports and audit

```sql
CREATE TABLE import_batches (
  id, source_filename, file_s3_key, file_sha256,
  feed_role,          -- primary | enrichment | reconciliation
  detected_grain,     -- signature id, see §4.1
  period_year, period_month,
  state,              -- uploaded|parsed|dry_run|committed|rolled_back|failed
  row_count, total_volume,
  supersedes_batch_id BIGINT REFERENCES import_batches(id),
  revision_reason TEXT,
  created_by, created_at, committed_at
);

CREATE TABLE change_log (
  id, entity_type, entity_id, action,
  before JSONB, after JSONB,
  volume_impact BIGINT,       -- units moved by this change, if computable
  actor_id, actor_kind,       -- human | agent | migration
  batch_id, created_at
);
CREATE INDEX ON change_log (entity_type, entity_id, created_at DESC);
```

Per `BUSINESS_LOGIC_PLAN.md` §7.1: **no locked periods, no approval workflow.** Any user may change anything. `change_log` is the sole control, so it must be written for *every* mutation without exception, including agent and migration actions.

---

## 3. The resolution engine

The core of the system. Pure Go, no I/O in the matching functions, fully unit-testable.

### 3.1 The ladder

Resolve **brand first, then model within that brand.** Stop at the first hit.

| Tier | Method | Configurable | Result status |
|---|---|---|---|
| 1 | Exact match on `alias.raw` | no | `confirmed` |
| 2 | Exact match on `alias.raw_normalized` | no | `confirmed` |
| 3 | Fuzzy within brand, score ≥ `fuzzy_threshold` | **yes** | `auto_resolved` |
| 3b | Fuzzy, score ≥ `fuzzy_review_floor` but < threshold | **yes** | `needs_review` (with proposal) |
| 4 | AI judgement, confidence ≥ `ai_threshold` | **yes** | `auto_resolved` |
| 4b | AI judgement below threshold | **yes** | `needs_review` (with proposal) |
| 5 | No candidate | — | `unresolved` |

Tiers 1–2 must run before any network call. They will handle >99% of rows and cost nothing.

### 3.2 Normalisation (tier 2) — specify exactly

Arabic text normalisation, applied identically when writing `raw_normalized` and when querying:

1. Unicode NFKC.
2. Strip tatweel `ـ` (U+0640) and all diacritics/harakat (U+064B–U+065F, U+0670).
3. Unify alef: `أ إ آ ٱ` → `ا`.
4. Unify yeh: `ى ي ئ` → `ي`.
5. Unify teh marbuta: `ة` → `ه`.
6. Unify waw: `ؤ` → `و`.
7. Convert Arabic-Indic digits `٠١٢٣٤٥٦٧٨٩` and Eastern `۰۱۲۳۴۵۶۷۸۹` → ASCII.
8. Lowercase Latin.
9. Collapse all whitespace runs to a single space; trim.
10. Remove `-`, `_`, `.`, and spaces **between a letter run and a digit run** (so `ZX 7`, `ZX-7`, `ZX7`, `T- 2` all collapse to `zx7` / `t2`).
11. Store a **second key with all whitespace removed** (`raw_normalized_nospace`) and match on it as **tier 2b**, after tier 2 and before fuzzy.

Step 10 solves the stated `ZX7 / ZX 7` case and the observed `T2 / T- 2`, `تيجو7 / تيجو 7` families.

**Step 11 was added after Phase 0 measured it.** Intra-word spacing in Arabic (`فور تشنر` vs `فورتشنر`) is not reachable by step 10 because no digits are involved. Stripping all whitespace was tested across the full historical dataset: it introduced **zero new ambiguous keys** while correctly unifying **14 additional groups** — `فور تشنر`/`فورتشنر`, `لاند كروزر`/`لاندكروزر`, `hr v`/`hrv`, `c s35`/`cs35`, `d fsk`/`dfsk`, `t f r`/`tfr`. Safe, and it removes 14 items from the review queue permanently.

Both keys are indexed. Match tier 2 first, then tier 2b. Test all of it explicitly — the Phase 0 regression suite (9 must-merge, 5 must-stay-apart) is the minimum bar.

### 3.3 Fuzzy matching (tier 3)

- Score = max of (a) normalised Levenshtein ratio, (b) token-set ratio, over the brand's model names **and** its existing aliases.
- Search space is **only** models under the already-resolved brand. Never global.
- **Guard against digit differences.** `X70` vs `X90` and `Tiggo 7` vs `Tiggo 8` score high on edit distance but are different cars. If the two strings' digit sequences differ, cap the score below `fuzzy_threshold` so it can never auto-link — route to review instead. This single rule prevents the most damaging class of error in this dataset.
- Suffix tokens that indicate a **variant, not a typo** (`plus`, `pro`, `max`, `fl`, `l`, `ev`, `hev`, `i-dm`, `gt`) must not be discarded during matching. `x70` and `x70 plus` are distinct nodes; a matcher that strips `plus` silently merges them.

### 3.4 AI resolution (tier 4)

Called **once per distinct unresolved string**, never per row. Batch all of a month's unresolved strings into one job.

**Request contract** — a strict JSON schema, enforced via tool-use so the model cannot return prose:

```jsonc
// input assembled by Go
{
  "raw_brand":  "شيرى",
  "raw_model":  "تيجو7 pro max",
  "resolved_brand": "Chery",
  "known_models": [ {"name":"Tiggo 7","aliases":["تيجو7","تيجو 7"]},
                    {"name":"Tiggo 7 Pro Max","aliases":[]}, ... ],
  "recent_rejections": [ {"raw":"تيجو 8","proposed":"Tiggo 7","reason":"different model"} ],
  "volume": 3
}
```

```jsonc
// required output
{
  "decision": "match_existing" | "new_model" | "uncertain",
  "model_name": "Tiggo 7 Pro Max",
  "confidence": 0.0,           // 0..1
  "reasoning": "one sentence, shown to the reviewer",
  "proposed_attributes": {      // only when decision = new_model
    "car_type": "Passenger", "segment": "Compact SUV", "tier": "Baseline",
    "engine_type": "ICE", "supply": "SUP"
  }
}
```

**Hard rules for the implementation:**

- `decision: "new_model"` is **only** permitted for models. A brand the agent has never seen always produces `uncertain` and queues for human creation — new brands are rare (~8/month) and each is a business event.
- `proposed_attributes` **always** lands as `needs_review`, never `auto_resolved`, whatever the confidence. A wrong segment silently distorts every segment report thereafter.
- Never overwrite a row whose current status is `confirmed` or `rejected`.
- Use **prompt caching** on the system prompt, segment taxonomy and per-brand model catalogue — they repeat across every call in a batch. Use **batch inference**; imports are not latency-sensitive. Both are supported on Bedrock (§11.4). Together these cut AI cost by ~70% (see `WEB_APP_PROPOSAL.md` §5.4).
- Called through **AWS Bedrock** using the ECS task's IAM role — there is no API key (§11.4). On failure: retry with backoff, then fall through to `unresolved`. **An AI outage must never fail an import** — the volume still lands, it just queues.

### 3.5 Settings

```
fuzzy_threshold      default 0.92   (conservative)
fuzzy_review_floor   default 0.75
ai_threshold         default 0.90   (conservative)
ai_model             default eu.anthropic.claude-sonnet-*  (Bedrock EU inference profile)
```

Stored in a `settings` table, editable in the UI, every change written to `change_log`. Persist each fact's resolution `confidence` so the settings screen can answer *"at this threshold, last month's import would have auto-linked N more rows"* — required by `BUSINESS_LOGIC_PLAN.md` §4.1.

---

## 4. Ingestion pipeline

### 4.1 Role and grain detection

On upload: read the header block, normalise the header strings, and match against a registry of known **signatures**. Each signature declares its role, its column mapping, and its grain.

Signatures to implement from the sample set (headers are on row 2, with a merged sub-header on row 3 where a dimension is pivoted across columns):

| Signature | Header columns | Role | Notes |
|---|---|---|---|
| `brands_models_by_status` | م, محافظة الإصدار, المنفذ, الماركة, الطراز, [Used, Zero, جمارك, هيئه دبلوماسية], الإجمالي العام | **primary** | Take the `Zero` column as volume |
| `brands_models_by_license_status` | م, محافظة الإصدار, نوع الترخيص, الماركة, [الموديل ×4 statuses], الإجمالي العام | enrichment | adds `license_type` |
| `zero_by_model_year` | م, محافظة الإصدار, المنفذ, الماركة, الموديل, [2023..2027], الإجمالي العام | enrichment | adds `model_year` |
| `zero_by_shape_cc` | الماركة, الموديل, الشكل, السعة اللترية, عدد المركبات | enrichment | attaches `body_shape`, `engine_cc` to the **tree**, not to facts |
| `ev_by_license_type` | الماركة, الموديل, [11–19 license types], Grand Total | reconciliation | validates the `BEV` flag |
| `insurance_duration` | #, المدة التأمينية, عدد المركبات, النسبة المئوية | **reject** | no join key |
| PDF (any) | — | reconciliation | totals only; see §4.5 |

**Two parsing traps present in every sample file:**

1. **Merged/sparse dimension cells.** `محافظة الإصدار` and `المنفذ` are written once and then left blank for every subsequent row of the same group. The parser must **forward-fill** these columns. A parser that reads them literally assigns thousands of rows to a blank governorate.
2. **Two-row headers.** Where a dimension is pivoted across columns (vehicle status, model year), row 2 holds the merged group label and row 3 holds the individual values. Parse both rows before mapping columns.

If no signature matches, **stop** and show the detected header to the user rather than guessing. Silent mis-parsing is the worst possible failure here.

### 4.2 Period detection

Try in order: (1) explicit period in the file header text (`عن الفترة من 2026/07/01 وحتى 2026/07/31`), (2) a `YYMMDD` prefix in the filename, (3) ask the user. Always display the detected period for confirmation before the dry run.

### 4.3 Stages

```
uploaded → parsed → dry_run → committed
                            ↘ rolled_back
```

**Dry run produces, and must display, all of:**

- rows read, total volume, detected period, detected signature
- resolution breakdown by tier (exact / normalized / fuzzy / AI / unresolved)
- count of new brands and new models proposed
- **comparison against the previous month's totals** — a >30% swing is flagged loudly, because it almost always means the wrong file
- comparison against any existing batch for the same period (this would be a revision)

Nothing is written to `facts` until commit.

### 4.4 Idempotency and revision

Re-importing a period **supersedes** the previous batch:

1. Compute a diff (rows added / removed / volume changed).
2. Show it. Prompt for an optional `revision_reason`.
3. On confirm: mark the old batch `rolled_back`, set `supersedes_batch_id`, insert the new facts.
4. Mark the period as **revised** and write to `change_log`. Any dashboard whose range includes a revised period shows a marker (§6.4).

### 4.5 Reconciliation feeds

PDFs and aggregate reports **never write facts**. They produce a variance report: extracted total vs computed total for the same period, per dimension where available. Deprioritise to Phase 3 — the sample PDFs are watermarked RTL with visually-ordered text extraction, high effort and low value.

---

## 5. Analytics layer

### 5.1 The one query that powers five dashboards

Every dashboard in the source workbook is the same shape: **rows = a dimension**, **columns = Jan…Dec**, then `TTL`, `Share %`, `YTD prior`, `YTD current`, `Growth %`, plus a prior-year share block and share-point delta.

Implement it **once** as a parameterised query:

```
GET /api/v1/analytics/matrix
  ?dimension=brand|model|segment|region|governorate|traffic_unit
             |distributor|car_type|engine|origin|supply|tier
  &year=2026 &compare_year=2025
  &filter.brand=MG,Chery &filter.car_type=Passenger,Bus,Commercial
  &filter.region=... &filter.segment=... &filter.month=...
  &limit=100
```

Returns rows of: `key, m1..m12, total, share_pct, ytd_current, ytd_prior, growth_pct, share_pct_prior, share_point_delta, rank, rank_prior`.

Adding a dimension must be a one-line change to a dimension registry, not a new endpoint.

### 5.2 Measures — required behaviours

- **Volume** is the only base measure (`SUM(facts.volume)`).
- **Share %** denominator = the **current filter context**. The API must return the denominator explicitly so the UI can display it; an unlabelled share percentage will be misread.
- **Growth %**: when the prior period is 0, return `null` with a reason code, and render "new" — **not** a number. The source workbook shows `502.67` growth for a brand that sold 3 units last year; do not reproduce that.
- **Share-point delta** is a first-class field, not a helper column. It is the metric the business competes on.
- **Rank and rank change** vs the comparison period.

### 5.3 Filters

All of: `brand, model, segment, tier, car_type, engine, origin, supply, distributor, region, governorate, traffic_unit, year, month` — plus, where enrichment supplied them, `license_type, vehicle_status, model_year, body_shape, fuel_type`.

**Filters are global**, held in one store, applied to every view, and encoded in the URL so a filtered view can be shared. The source workbook's biggest usability failure is per-pivot disconnected slicers; do not reproduce it.

### 5.4 The default-filter trap — critical

The source workbook's headline figure of **51,252** for Jan–Feb 2026 is exactly `Passenger + Commercial + Bus`, behind a hidden slicer that also excluded 58,819 motorcycles and 2,220 `Undefined`.

Motorcycles are now out of scope by decision (§0.1), so that half of the trap is gone: the app's natural total for the period is **53,494**. What remains is `Undefined` (2,220 units) and `Construction` (22) — small, but they must not vanish silently.

Requirements:

- If any default filter is active, the UI must display it as a **visible, labelled, removable chip**. Never a hidden default.
- Show both figures where a default excludes data: "51,252 shown · 53,494 total".
- Ship with **no default filter**, and let the user save a preset if they want the old behaviour.

### 5.5 Data-confidence indicator

Every dashboard displays the composition of the data behind it: `% confirmed / % auto-resolved / % needs review / % unresolved`, for the current filter and period. Reports never block on the review queue — they disclose.

### 5.6 Performance

With ~530k rows growing to ~2M over ten years, plain Postgres aggregation is sufficient. Index on `(period_year, period_month)`, `(brand_id, period_year, period_month)`, `(model_id, …)`. Add a materialised view only if a measured query exceeds 500 ms. **Do not pre-optimise** — an unnecessary aggregation layer is a second source of truth and a permanent bug farm.

### 5.7 Excel export

Export the matrix view in the current workbook's column layout. This is not a nice-to-have: downstream habits are built on that format, and a familiar export is what buys permission to change the process.

---

## 6. Frontend (React)

### 6.1 Screens

| Route | Purpose |
|---|---|
| `/import` | Upload, detected role/period confirmation, dry-run report, commit, batch history |
| `/review` | The queue. Volume-ranked, agent proposal + reasoning, Confirm / Reject / Reassign, bulk actions |
| `/tree` | Brand → Model browser, attribute editing, alias panel, merge/split, change history |
| `/tree/settings` | Segments, tiers, distributors + effective dating, resolution thresholds |
| `/dashboard/:dimension` | The matrix view — brand, model, region, segment, distributor |
| `/geography` | Region → Governorate → Unit tree and its aliases |

### 6.2 The review queue is the product

It should feel like an inbox to be emptied, not a table to be browsed.

- Sorted by **volume impact** descending, always.
- Each item: raw string, agent proposal, confidence, one-sentence reasoning, volume, source period.
- Keyboard-first: `Enter` confirm, `R` reject, `/` search to reassign, `J/K` navigate. A user should clear 85 items in minutes.
- **Bulk confirm** by confidence band ("all 43 above 95%").
- **Optimistic concurrency**: if two users act on the same item, the second is told it was just resolved rather than having their decision silently discarded (§ shared inbox, `BUSINESS_LOGIC_PLAN.md` §5).

### 6.3 Tree editing safety

- **Impact preview before every destructive save.** "This merge moves 12,400 units from Crossover to Compact SUV." Editing the tree rewrites history; the user must see that before committing.
- Merge and split both reversible; split re-adjudicates affected aliases rather than pretending to undo.
- Volume shown at every node so the user knows what matters — fixing a 40,000-unit node beats fixing a 3-unit one.

### 6.4 Revision markers

Any dashboard whose date range includes a revised period shows a marker with one-click access to what changed, when and by whom.

---

## 7. API surface

```
POST   /api/v1/imports                      upload → batch
GET    /api/v1/imports/:id/dry-run          resolution preview + variance
POST   /api/v1/imports/:id/commit
POST   /api/v1/imports/:id/rollback
GET    /api/v1/imports                      history

GET    /api/v1/review?status=needs_review&sort=volume
POST   /api/v1/review/:id/confirm
POST   /api/v1/review/:id/reject
POST   /api/v1/review/:id/reassign
POST   /api/v1/review/bulk-confirm          {min_confidence}

GET    /api/v1/brands  /models  /segments  /distributors
PATCH  /api/v1/models/:id
POST   /api/v1/models/:id/merge             {into_id}  → returns impact preview first
POST   /api/v1/models/:id/split
GET    /api/v1/models/:id/aliases
DELETE /api/v1/aliases/:id

GET    /api/v1/analytics/matrix             §5.1
GET    /api/v1/analytics/timeseries
GET    /api/v1/analytics/confidence
GET    /api/v1/analytics/export.xlsx

GET    /api/v1/settings   PATCH /api/v1/settings
GET    /api/v1/changes?entity_type=&entity_id=
```

Long-running operations (import, resolve, migration) return a job id; progress streams over SSE.

---

## 8. Migration (Phase 0) — ✅ EXECUTED

**Phase 0 has been run. Its output is in `phase0/seed/*.csv` (11 files) and `phase0/PHASE0_REPORT.md`.**
The build agents should **load those CSVs**, not re-derive them. The steps below document how they were produced and remain the spec for re-running if the source workbook is reissued.

Measured result: seeded aliases resolve **98.04% of rows at brand level and 96.80% at brand+model level** using deterministic tiers 1–2 alone, with no fuzzy matching and no AI.

Produced: **201 brands · 1,243 models · 13 segments in use · 315 brand aliases · 2,510 model aliases · 27 governorates · 210 traffic units · 138 distributor assignments · 115 review items.**

A one-shot command, re-runnable, that reads the source workbook and produces the seed state.

1. **Apply the §0.1 scope rules first** — drop motorcycles, consolidate brand casing — then extract dimensions from the surviving `Consolidated` values → brands, models, segments, engine, supply. Brand→Origin from the `Name` sheet `N:O` columns. Geography from `Region & Traffic Area` and the unit list — *rebuilt explicitly, not row-aligned* (§2.3).
2. **Normalise known collisions.** `ICE`/`ICe`/`Ice`, `HYBRID`/`Hybrid`, `Undefined`/`Undefinded`, `Mid-Bus`/`Mid Bus`, `MPV`/`MPV-B`, `HB-B`/`Car HB-B`, `SUV-3`/`Luxury SUV-3`, and trailing-whitespace brands (`Cadillac `).
3. **Split `Luxury ` prefix → `tier`.** Prefix present → `Highline`, absent → `Baseline`; strip from segment name. Seed only — the business reviews and corrects (`BUSINESS_LOGIC_PLAN.md` §3.1.2).
4. **Seed aliases.** Every distinct `(raw_brand → Brand)` and `(brand, raw_model → Model)` pair from the existing data becomes a `confirmed` alias. This is why steady-state AI volume is ~200 calls/month rather than thousands.
5. **Seed distributor assignments** from `List Distributor Lookup`, all `valid_from = 2021-01-01`, `valid_to = null`.
6. **Import all 377,425 car facts** as batches, one per period, recording the 148,921 dropped motorcycle rows against each batch. Do **not** clean values during import — `#N/A`, `Undefined`, `Other` and `0` land as `unresolved` and populate the queue, volume-ranked.
7. **Resolve conflicts by volume**, flagging each: where one model carries conflicting segments across rows, take the highest-volume value and mark `needs_review`.

### 8.1 Migration acceptance tests — must pass

```
✓ raw rows read from source                 = 526,346
✓ raw volume                                = 2,246,287
✓ motorcycle rows dropped                   = 148,921        (1,138,473 units)
✓ car fact row count                        =   377,425
✓ car volume after migration                = 1,107,814
✓ RECONCILIATION  1,107,814 + 1,138,473     = 2,246,287      <- must hold exactly
✓ per-period volume                         matches source workbook, all 60 periods
✓ 2026 natural-key uniqueness               17,600 rows, 17,600 distinct keys, 0 duplicates
✓ Jan+Feb 2026, car_type in {Passenger,Commercial,Bus}  = 51,252   (matches Brand Dashboard)
✓ Jan+Feb 2026, all retained car types                  = 53,494
✓ 2026 by car type: Passenger 40,701 · Commercial 7,567 · Bus 2,984
                    · Undefined 2,220 · Construction 22
✓ canonical brands / models / segments      = 201 / 1,243 / 13 in use
✓ distinct raw (brand,model) pairs          = 3,779
✓ distinct raw brand strings                =   781
✓ distinct raw traffic-unit strings         =   222
✓ brand case changes: 14 merged · 18 re-cased · 29 acronyms held
```

> A migration that "cleans up" totals is a **broken** migration. Totals must match the source exactly; cleaning happens afterwards, in the app, visibly.

---

## 9. Non-functional

| Area | Requirement |
|---|---|
| **Auth** | Email + password, argon2id, admin seeder — see §11.5. No roles; every action attributed in `change_log`. |
| **Audit** | Every mutation written to `change_log`, no exceptions, including agent and migration actions. |
| **Backups** | RDS automated backups, 30-day retention. Weekly logical dump of the **tree tables** to S3 — the classification is the irreplaceable asset; facts can be re-imported. |
| **Secrets** | Database credentials via Secrets Manager or SSM Parameter Store. **No AI key exists** — Bedrock authenticates by IAM task role (§11.4). |
| **Observability** | Structured JSON logs to CloudWatch. Metrics: import duration, resolution tier distribution, AI calls + tokens + cost per batch, queue depth. Billing alarm at 2× expected. |
| **i18n** | UI in English; **all data display must handle RTL Arabic correctly**, including in tables and exports. Store text as UTF-8 throughout; never transliterate raw strings. |
| **Testing** | Normalisation and fuzzy matching require exhaustive table-driven unit tests — they are the correctness core. Include every real collision found: `ZX7/ZX 7`, `T2/T- 2`, `تيجو7/تيجو 7`, `فورتشنر/فور تشنر`, `x70/x70 plus/X70 FL`, `ICE/ICe/Ice`. Golden-file test for the migration. |

---

## 10. Build order

| Phase | Deliverable | Definition of done |
|---|---|---|
| **0** | ✅ **DONE** — seeded trees in `/seed` (15 CSVs) | §8.1 acceptance tests all pass |
| **1** | Import → resolve → review → tree | A real month committed end-to-end in **under an hour** |
| **2** | Matrix analytics, filters, Excel export | Reproduces every figure in the source workbook, all 60 periods |
| **3** | Enrichment feeds, reconciliation, distributor effective-dating, time series | — |
| **4** | Duplicate scanning, agent learning from queue decisions, anomaly alerts | — |

**Do not begin Phase 2 before Phase 1's review queue works.** Dashboards over unresolved data reproduce the existing problem in a new colour scheme.

---

## 11. Deployment target — decided and approved

**Option A− from `WEB_APP_PROPOSAL.md` §5.2, ~$56–60/month, Frankfurt (`eu-central-1`).** Build to exactly this. Do not add components not listed here.

> **Edge redesign (2026-08-23):** Cloudflare is dropped. Ingress is now **CloudFront**
> (TLS at the edge via ACM, Shield Standard) with the **SPA split to S3** (`/*`) and the
> **API served from the Fargate task** (`/api/*`) through a **t4g.nano Caddy proxy** that
> is the only public origin. This **reverses the earlier "no CloudFront / Cloudflare Tunnel"
> decision** below. The proxy replaces what the Tunnel did (stable origin, locked ingress)
> without an ALB; the Fargate task registers in **AWS Cloud Map** so its ephemeral IP does
> not break the origin. Roughly cost-neutral vs. the Tunnel plan — the win is removing the
> Cloudflare dependency and gaining a real CDN + independent front-end deploys.

| Component | Exact spec | Monthly |
|---|---|---:|
| ECS Fargate | **1 task**, 0.5 vCPU / 1 GB, **ARM64 (Graviton)**, 24/7 (+ public IPv4 ~$3.6) | $21.6 |
| RDS PostgreSQL 16+ | **db.t4g.small**, Single-AZ, 7-day automated backups | $23.36 |
| Storage | 20 GB gp3, autoscaling to 100 GB | $2.30 |
| Ingress | **CloudFront** → S3 (SPA) + **t4g.nano Caddy proxy** (EIP) → Fargate; no ALB | ~$8 |
| S3 | SPA + uploads, exports, weekly tree dumps; IA lifecycle at 90 days | ~$2 |
| CloudWatch / Secrets / ECR / Route53 | logs at **7-day retention** | ~$3 |
| **Total** | **Frankfurt ~$56–60** | |

Ingress is a Terraform flag, `use_proxy` (**deployed value: `false`**):
- `true` → a t4g.nano Caddy box (EIP) is the CloudFront `/api` origin.
- `false` (~$7/mo cheaper, ~$50) → **no box**; a Lambda fired by EventBridge on each ECS
  task reaching RUNNING writes the task's **public** IP into a Route53 record that CloudFront
  uses as the `/api` origin on :8080. (Needed because ECS service discovery only publishes the
  task's *private* IP, unreachable from the CloudFront edge.) Tradeoffs: a brief 5xx window
  during task recycles while Route53 + CloudFront re-resolve, and a plain-HTTP CloudFront→origin
  hop — the same posture as the proxy path; only an ALB+ACM cert (~$16/mo) can encrypt that hop.
  The viewer→CloudFront leg is TLS in both cases.

### 11.1 Account, region and infrastructure-as-code

- **Existing AWS account**, region **`eu-central-1` (Frankfurt)**.
- **Terraform**, using a **named AWS profile** — never hard-coded credentials, never a key in the repo. Provider block reads `profile` from a variable so the same code runs against a different account unchanged.
- State in **S3 with DynamoDB locking**, in the same account. Bootstrap the backend as step one.
- All infrastructure in the repo. The cost profile above must be reproducible from source, not clicked together in the console.
- **Billing alarm at $115/month** (2× expected) from day one.

### 11.2 Explicitly excluded — do not add

| Excluded | Would cost | Why not |
|---|---:|---|
| Application Load Balancer | $16.43/mo + LCU | **Approved: removed.** A t4g.nano Caddy proxy is the stable CloudFront origin at ~$4/mo instead. The task SG admits only the proxy; the proxy SG admits only CloudFront's managed prefix list — zero open public ingress. |
| NAT Gateway | $32.85/mo **per AZ** | Task in a public subnet with a locked-down security group (egress via IGW); VPC endpoints for S3 and Bedrock. |
| Second Fargate task | $18/mo | Worker is a goroutine (§1). |
| Multi-AZ RDS | +$23/mo | Single-AZ + automated backups until downtime has a measured cost. |
| Aurora Serverless v2 | ~$43/mo floor | Does not scale to zero; more expensive than `db.t4g.small` here. |
| Redis / SQS / Kafka | varies | Job queue is a Postgres table (§1). |
| Secrets Manager for the AI key | ~$0.40/mo | **Not needed** — Bedrock authenticates by IAM task role (§11.4). |

### 11.3 Repository

**`https://github.com/Mina-Nabil/mc-sales-analysis`** — new, private.

```
/cmd/server          main.go (API + worker goroutine + --worker flag)
/internal/domain     car tree, aliases, resolution engine (pure, no I/O)
/internal/ingest     parsers, signature detection, dry-run
/internal/analytics  the matrix query (§5.1)
/internal/ai         Bedrock client, prompts, schemas
/internal/store      sqlc-generated queries + migrations
/web                 React app; build output embedded via embed.FS
/infra               Terraform
/seed                the 15 Phase 0 CSVs — committed, they are the seed state
/migrations          golang-migrate SQL
```

CI: build, vet, `go test ./...`, Docker build for `linux/arm64`, push to ECR. The normalization and fuzzy-matching table tests (§9) must gate the build — they are the correctness core.

### 11.4 AI access — AWS Bedrock

**Claude via Amazon Bedrock in `eu-central-1`, not the Anthropic API directly.**

- **Auth is the ECS task's IAM role.** No API key exists, so nothing to store or rotate — this is why Secrets Manager is not in the stack.
- Use the **EU cross-region inference profile** (`eu.anthropic.claude-*`) so requests stay within EU regions; the plain regional model id may not be available in Frankfurt for every model.
- **Model: Claude Sonnet** (per `WEB_APP_PROPOSAL.md` §5.4). Make the model id a setting (§3.5) so it can be changed without a deploy.
- **Bedrock on-demand pricing matches Anthropic list pricing**, and Bedrock supports both **prompt caching** and **batch inference** at a 50% discount — so the ~$0.50/month estimate holds. Verify against the Bedrock price page for `eu-central-1` before the first invoice, since regional rates can differ.
- IAM policy: `bedrock:InvokeModel` and `bedrock:InvokeModelWithResponseStream` on the specific inference-profile ARN only. Not `*`.
- Add a **VPC endpoint for Bedrock** so calls do not need internet egress — this is what keeps the NAT Gateway out of the design.
- The §3.4 failure rule is unchanged: on a Bedrock error, retry with backoff, then fall through to `unresolved`. **An AI outage must never fail an import.**

### 11.5 Authentication

**Email + password.** No SSO.

- Passwords hashed with **argon2id** (or bcrypt cost ≥ 12). Never anything faster.
- Sessions: HTTP-only, Secure, SameSite=Lax cookies. Server-side session records so a session can be revoked.
- **No roles.** Every authenticated user can do everything (`BUSINESS_LOGIC_PLAN.md` §5). Control is attribution — every action writes to `change_log`.
- **Admin seeder**: a `--seed-admin` command on the same binary, creating the first user from `ADMIN_EMAIL` / `ADMIN_PASSWORD` env vars. Idempotent — re-running updates the password rather than erroring. It must **refuse to run if any user already exists** unless `--force` is passed, so it cannot become a backdoor.
- Users are created in-app by any existing user; there is no public registration.
- Rate-limit login attempts per email and per IP.

### 11.6 Motorcycles — excluded by configuration, not by deletion

The importer **retains full motorcycle parsing**. Exclusion is a setting:

```
ingest.exclude_motorcycles = true   (default)
```

- With it on, motorcycle rows are parsed, counted, and **reported as dropped** on every dry-run and commit — never silently discarded — then not written to `facts`.
- Turning it off must be sufficient to start ingesting motorcycles again. No code change, no migration, no re-architecture.
- The car tree already carries `Motorcycle` and `Scooter` in the segment vocabulary and `Motorcycle` in car type, so the classification survives even though no current row uses it.

### 11.7 Guardrails

- **ARM64/Graviton** for both container and database. ~20% cheaper, identical performance; build the Go binary for `linux/arm64`.
- **Reserved Instances deferred.** Revisit after 3 months of measured usage — a 1-year RDS RI takes `db.t4g.small` from $23.36 to $17.01.
- **Weekly logical dump of the tree tables to S3**, separate from RDS snapshots. The classification is the irreplaceable asset; facts can be re-imported from source files.
