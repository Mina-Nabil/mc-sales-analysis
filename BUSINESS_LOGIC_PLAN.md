# Egypt Car Sales Analytics — Business Logic Plan

**Stage 1 deliverable: what the system must do. No technology decisions.**
**Status: business decisions resolved (§8) · Phase 0 executed · scope narrowed to cars only.**

> **SCOPE — decided by the business, applied everywhere in this document:**
> 1. **Motorcycles excluded entirely** — 148,921 rows / 1,138,473 units (50.7% of the raw file) dropped at ingestion.
> 2. **Brand and model names case-consolidated** — names of **4 letters or fewer are ALL CAPS** (`KIA`, `SEAT`, `FIAT`, `MG`, `BMW`, `BYD`); longer all-caps names become Title case (`VIGOREY` → `Vigorey`).
> 3. **Segments are body types, not size codes** — 37 size-coded segments collapsed to 13 in use (§3.1.3).
> 4. **Distributor is optional** — many brands have none, and that is a permanent valid state, not a gap.
> 5. **No map** — coordinates are not stored.
> 6. **Open questions researched, not delegated** — generation splits, brand renames and sub-brand relationships were settled from published sources (§8.4). The review queue fell from 191 items to 115.
>
> All figures below are **cars only**: **377,425 rows / 1,107,814 units**.

Derived from analysis of:

- `260308_Registration Report Dashboard Inc Distributor.xlsx` — the target output (377,425 fact rows, 1,107,814 units, Feb-2021 → Feb-2026, 60 periods)
- `mainsource.zip` — 9 Excel + 5 PDF reports from the traffic authority, all for Jul-2026

---

## 1. What the data actually is

### 1.1 The output workbook is not a report — it's a manual ETL pipeline

The target workbook's hidden `Raw Data` sheet is the real product. It has 20 columns and one row per fact:

| # | Column | Nature | Source |
|---|---|---|---|
| A | محافظة الإصدار | **raw** — Arabic governorate | traffic authority file |
| B | المنفذ | **raw** — Arabic traffic unit | traffic authority file |
| C | الماركة | **raw** — Arabic brand | traffic authority file |
| D | الطراز | **raw** — Arabic model | traffic authority file |
| E | Govererate | derived — EN governorate (27) | manual mapping |
| F | Region | derived — EN region (8) | manual mapping |
| G | Unit | derived — EN traffic unit (206) | manual mapping |
| H | Consolidated | derived — `Brand-Model` | formula `CONCATENATE(J,"-",K)` |
| I | Car Type | derived — 6 values | **manual mapping** |
| J | Brand | derived — EN brand (365) | **manual mapping** |
| K | Model | derived — EN model (1,457) | **manual mapping** |
| L | Segment | derived — 47 values | **manual mapping** |
| M | Engine | derived — ICE/BEV/HYBRID/REEV… | **manual mapping** |
| N | CKD SUP | derived — locally assembled vs imported | **manual mapping** |
| O | Vol | **measure** — unit count | traffic authority file |
| P | Month | **raw** — report period | filename / report header |
| Q | Origin | derived — country of origin (10) | formula `VLOOKUP(Brand, Name!N:O)` |
| R | Year | **raw** — report period | filename / report header |
| S | Brand and type | derived — `CarType-Brand` | formula |
| T | Distributor | derived — 39 distributors | formula `VLOOKUP(Brand and type, …)` |

**Only 7 of 20 columns come from the source files. The other 13 are enrichment.** Nine of them are pasted static values — i.e. a human re-does that mapping work every single month. That manual step is what the app replaces.

### 1.2 The fact grain is proven

For 2026 the key `(محافظة الإصدار, المنفذ, الماركة, الطراز, Month, Year)` gives **23,451 distinct keys across 23,451 rows — zero duplicates**. This is the canonical fact grain:

> **one row = one (governorate × traffic unit × raw brand × raw model × month) → volume**

Everything else in the workbook is a projection of this.

**And the repetition is measurable.** Across the whole history there are only **3,779 distinct raw (brand, model) pairs** and **781 distinct raw brand strings**. Tracking how many pairs appear for the first time in each month:

| | Last 12 months, average |
|---|---:|
| Fact rows arriving per month | 8,031 |
| Of those, **pairs never seen before** | **64** |
| New brand strings per month | 7 |

> **99.2% of each month's mapping work is a repeat of a decision already made.** That single ratio is the justification for the alias mechanic in §3.1.1 and for why AI cost stays negligible (§4.1) — the agent is asked about ~64 things a month, not 8,000.

**Phase 0 confirmed this empirically.** Replaying all five years against the seeded alias library, using only the deterministic tiers with no fuzzy matching and no AI, resolves **98.0% of rows at brand level and 96.8% at brand + model.**

### 1.3 The existing data is dirty — but less than it first appeared

Scanning all 377,425 car rows:

| Problem | Rows | Units | % of volume |
|---|---:|---:|---:|
| `Distributor` = `Undefined` | 15,887 | 24,130 | 2.2% |
| `Distributor` = `#N/A` (lookup failed) | 10,225 | 23,497 | 2.1% |
| `Model` = `Undefined` / `Other` | 10,296 | 20,594 | 1.9% |
| `Car Type` = `Undefined` | 8,479 | 17,663 | 1.6% |
| `Segment` = `Undefined` / `Undefinded` | 8,478 | 17,665 | 1.6% |
| `Origin` = `0` (brand not in the 52-brand list) | 8,531 | 17,547 | 1.6% |
| `Brand` = `Undefined` | 8,414 | 17,381 | 1.6% |
| `CKD SUP` = `Undefined` | 7,374 | 15,604 | 1.4% |
| `Region` blank or formula error | 5,284 | 12,230 | 1.1% |

> ⚠️ **An earlier draft of this plan reported these as 158,529 / 145,475 / 112,372 rows.** Those figures included motorcycles. With motorcycles out of scope, the `#N/A` distributor problem shrinks from 1,160,394 units to **23,497** — because the distributor lookup was always built for cars and simply had no motorcycle entries. The remaining defects are real but each sits near 1–2% of volume, not half of it.

Plus inconsistency inside the controlled vocabularies, which is what actually corrupts grouped reports:

- **Engine**: `ICE` / `ICe` / `Ice` are three values; `HYBRID` / `Hybrid` are two.
- **Segment**: 45 distinct raw values, reduced to **37** after case-merging and extracting `Luxury ` into `Tier`.
- **Brand**: `Cadillac ` with a trailing space was its own value; all-caps duplicates like `BAIC`/`Baic`, `SOUEAST`/`Soueast` split their own volume.

> The honest business case is **not** "your data is broken". It is: the process is manual, repetitive, and unversioned, and the classification that drives every report exists nowhere but in one person's memory. The defects above are the symptom.

### 1.4 The Segment / Car Type mapping table does not exist in the file

`Origin` and `Distributor` have formulas pointing at lookup sheets in the workbook. `Car Type`, `Segment`, `Brand`, `Model`, `Govererate`, `Region`, `Unit`, `Engine`, `CKD SUP` are **pasted values with no formula and no lookup table**. The workbook also has a broken external link to a missing file (`TTL Region`).

**Implication:** the single most valuable asset in the business — the Brand→Model→Segment classification — currently lives only in someone's head plus 3,779 pasted brand-model strings. Phase 0 of this project is to reconstruct it from the existing data and make it a first-class, owned, versioned object. That reconstruction is a deliverable in its own right.

---

## 2. Input files — which ones matter

All 14 source files are **pre-aggregated pivots** from the traffic authority, each a different slice of the same underlying registration ledger for the same month. They overlap heavily. Accepting all of them naively guarantees double-counting.

### 2.1 Grain map

| File | Grain | Rows | Verdict |
|---|---|---|---|
| `إحصائية الماركات والطرازات وفقاً لحالة المركبة` | Governorate × **Unit** × Brand × Model × VehicleStatus | 71,759 | **PRIMARY FEED** |
| `…وفقاً لنوع الترخيص وحالة المركبة موزعة على محافظات` | Governorate × **LicenseType** × Brand × Model × VehicleStatus | 31,341 | **SECONDARY** — adds license type |
| `إحصائية الماركات والطرازات للمركبات الزيرو` | Governorate × Unit × Brand × Model × **ModelYear** | 13,016 | **SECONDARY** — adds model year |
| `…للملاكي الزيرو..` | same, private cars only | 8,247 | subset — reject |
| `…للملاكي الزيرو وفقاً للمحافظات..` | Governorate × Brand × Model | 3,033 | subset — reject |
| `…للملاكي الزيرو وفقاً للشكل والسعة اللترية` | Brand × Model × **BodyShape** × **EngineCC** | 1,367 | **ENRICHMENT** — feeds the car tree, not the fact table |
| `إحصائية ماركات المركبات الكهربائية…` (×2) | Brand × Model × LicenseType, EV only | 1,025 / 242 | **VALIDATION** — cross-check the BEV flag |
| `عدد المركبات… المدة التأمينية` | Insurance duration only | 11 | **REJECT** — no join key |
| 5 × PDF (`_watermark`) | LicenseType-level totals: fuel type, vehicle status, model year, unit productivity, top brands | — | **VALIDATION / CONTEXT** only |

### 2.2 The recommendation

**Support three input roles, not fourteen file types:**

1. **Primary feed** — the one file that reproduces the fact grain. Ingesting it *replaces* the entire month.
2. **Enrichment feeds** — files that carry attributes the primary feed lacks (body shape, engine CC, model year, license type, EV flag). These *never* create facts; they only attach attributes to car-tree nodes or add dimension breakdowns.
3. **Reconciliation feeds** — PDFs and aggregate reports. These *never* write anything. They produce a variance report: "your ingested Jul-2026 total is 236,324; the authority PDF says 236,324 ✓" or "…says 236,890 ✗ — 566 units unaccounted for".

Every uploaded file must be classified into one of these three roles **before** any data is written. The system should detect the role automatically from the header signature and ask for confirmation when unsure — but the roles themselves are a fixed, small set. This is the single most important design decision in the whole project: it converts "accept anything" into "accept anything, but know what it is".

### 2.3 Why "the more concise, the better" is right

Given the grain map, **one file per month is sufficient** to reproduce the entire output workbook. The rest add optional depth. Build the primary path first and make it excellent; enrichment feeds are additive and can land later without rework.

### 2.4 Note on PDFs

The PDFs are watermarked, right-to-left, and their extracted text is visually ordered rather than logically ordered. They are recoverable but are the highest-effort, lowest-value input in the set. Treat PDF as reconciliation-only and deprioritise it. If the authority also publishes the same report as Excel, always prefer the Excel.

---

## 3. The domain model

Four trees and one fact table. Everything the app does is CRUD on the trees plus slicing the facts.

### 3.1 The Car Tree (the core asset)

```
Brand                       e.g. "MG"
 ├─ aliases                 ["ام جـى", "أم جي", "MG Motor", "M G"]
 ├─ origin                  China            ← brand-level
 ├─ default distributor     per Car Type
 └─ Model                   e.g. "MG5"
     ├─ aliases             ["5", "MG 5", "ZS7"…]
     ├─ car type            Passenger | Commercial | Bus | Motorcycle | Construction
     ├─ segment             Sedan | Hatchback | Crossover | Compact SUV | SUV | MPV | …
     ├─ tier                Highline | Baseline        ← see 3.1.2
     └─ Variant  (optional, phase 2)
         ├─ body shape      Hatchback | Limousine | …   ← from the shape/CC file
         ├─ engine CC       1498
         └─ model year      2027
```

#### 3.1.1 Aliasing is the central mechanic

The user's example — *"ZX7 is the same car as ZX 7"* — is the whole problem in miniature. The evidence backs this up hard: the raw data contains `تيجو7` / `تيجو 7` / `تيجو7 pro max`, `x70` / `x70 plus` / `X70 FL`, `فور تشنر` / `فورتشنر`, `T2` / `T- 2` / `T2 i-DM`.

Rules:

- An **alias** is a raw string (Arabic or Latin, any spelling) that resolves to exactly one canonical node.
- Aliases are **owned by the node**, not by the import. Once `ZX 7 → ZX7` is confirmed, every past and future import uses it.
- Aliases are **never destructive**. The raw string is always retained on the fact row (columns A–D above). Re-mapping an alias re-derives history without re-importing anything.
- **Merging** two models moves all aliases and facts to the survivor and leaves a redirect. This must be reversible.
- **Splitting** is the hard case and must be supported: `X70` and `X70 Plus` were merged, and now the business wants them separate. Splitting requires re-adjudicating the affected raw strings — treat it as bulk re-assignment, not as an undo.

#### 3.1.2 Tier (Highline / Baseline)

**DECIDED: Tier is a model-level attribute, separate from segment.** `Segment = Compact SUV` and `Tier = Highline` are orthogonal, both hanging off the Model node.

- **Migration:** derived from the old `Luxury ` segment prefix (present → Highline, absent → Baseline). **Executed.**
- **The prefix is the seed, not the rule.** Tier is independently editable thereafter, with no dependence on segment naming. Expect corrections — the prefix was applied inconsistently in the source.
- **A brand can span both tiers.** Toyota Land Cruiser = Highline, Toyota Corolla = Baseline. There is no brand-level Tier field; "Mercedes is a highline brand" is a *derived* rollup, not a stored attribute.
- New models get a Tier proposal from the agent, always landing as **Needs review** (§4.4).

✅ **The segment rewrite in §3.1.3 fixed the problem this used to have.** Under the old size codes, highline and baseline lived on disjoint scales (`Car-3` vs `Car-C`), so tier and segment could never be crossed. Now that size is gone, **6 of the 13 in-use segments carry both tiers** and *"what share of Compact SUV is highline?"* is answerable.

#### 3.1.3 Segment — body type, not size

**DECIDED: segments describe body type. Size is dropped entirely; premium-ness lives in Tier.** 37 historical size-coded segments collapse to **13 in use**, from a controlled vocabulary of 24.

| Rule | Result |
|---|---|
| Every A/B/C/D/E/F-segment car | **Sedan** |
| All hatchback sizes | **Hatchback** |
| Coupe SUV · 7-seat SUV · 3-row SUV · Performance SUV · Luxury SUV · Off-road SUV | **SUV** |
| Crossover Coupe | **Crossover** |

The remaining in-use values name themselves: **MPV, Van, Pickup, Truck, Minibus, Bus, Sports car, Construction**.

**The SUV split was researched, not derived.** The source file's size codes proved unreliable — it coded 7-seat midsize vehicles and subcompacts alike as `SUV-C`. So the actual 150 models making up 97% of SUV volume were researched individually (length, wheelbase, seat rows):

| Bucket | Criteria |
|---|---|
| **Crossover** | A/B-segment — under ~4,400 mm, or a B-segment platform |
| **Compact SUV** | C-segment — ~4,400–4,700 mm, 2 rows |
| **SUV** | D/E/F-segment — over ~4,700 mm, **or** any 3-row / 7-seat model |

Platform beats length (Škoda Karoq at 4,390 mm is Compact SUV on the C-segment MQB platform); seat rows beat length (Toyota Rush at 4,435 mm is a 7-seater, so SUV); and premium brands get no exception (Mercedes GLC at 4,749 mm is SUV, not Compact SUV).

**50 models moved — 98,619 units, 23% of SUV volume.** Five were not SUVs at all: Peugeot 408 and Citroën C4X are fastback sedans; Citroën C4, DS 4 and Zeekr 001 are hatchbacks.

The vocabulary also carries **Coupe, Convertible, Roadster, Station Wagon, Liftback, Minivan, Microcar, Limousine, Supercar, Hypercar, Off-roader** — unused because the source does not distinguish body style that finely, but available for users and the agent going forward.


#### 3.1.4 Local assembly and engine type live on the FACT, not the model

Measured, not assumed. A model routinely moves between imported (SUP) and locally assembled (CKD) — Hyundai Elantra AD was imported through 2023 and locally assembled from 2024, when GB Auto started building it in May of that year. Proton Saga, Geely Coolray, Citroën C4X, Jetour X70 Plus and Foton View all show the same clean switch; Kia Sorento went the other way.

**67% of the volume in what looked like "model attribute conflicts" was simply this.** Storing `supply` and `engine_type` on the model node manufactures a conflict out of an ordinary commercial fact. They belong on the fact row, where they remain fully filterable but never need adjudicating.

#### 3.1.5 Sub-brands are separate brands with a parent link

Jetour, Exeed and Omoda are Chery marques; Denza is BYD's; IM is MG's; Deepal is Changan's; DS is Citroën's; Cupra is SEAT's; Zeekr is Geely's. Each carries its own badge and Egyptian price list, and **Omoda, IM, Zeekr and Deepal each have a different distributor from their parent** — so merging them would destroy the distributor reporting the business depends on.

Keep them as separate brands with an optional `parent brand` link, which gives "Chery Group" roll-ups for free. A **rename** is different from a parent link: SsangYong became KGM in March 2023 — same company, so one brand record with an alias, and the history trends as a single line.

### 3.2 The Geography Tree

```
Region (8)          Cairo, Giza, Alexandria, Delta, Upper Egypt, Canal, Red Sea, Sinai
 └─ Governorate (27)
     └─ Traffic Unit (206)
         ├─ aliases        ["وحدة مرور المقطم", "وحده مرور المقطم"]
```

Same alias mechanic as the car tree. 222 Arabic unit names map to 210 English units, so aliasing already happens here too. **No coordinates are stored — a map is out of scope.**

⚠️ **Data-quality warning found:** in the existing `Name` sheet, `وحدة مرور اسيوط` (Assiut, Upper Egypt) sits in a row where the Region column reads `Canal`. The Region/Governorate/Unit columns in that sheet are three *independent* lists that happen to share rows, not a joined table. Do not port that sheet's row alignment as if it were a relationship — rebuild the hierarchy explicitly.

### 3.3 The Commercial Tree

```
Distributor (39)     Mansour Auto, GB Auto, Nissan Egypt, Kayan, EIM/EIT…
 └─ assignment: (Brand, Car Type) → Distributor
```

Assignment is by **brand + car type**, not brand alone — the existing lookup keys on `Passenger-Chery` vs `Bus-Chery`.

**DECIDED: agency changes are real, so assignment is effective-dated from day one.** Each `(Brand, Car Type) → Distributor` assignment carries a valid-from / valid-to period. A fact row resolves its distributor using **the assignment in force during the fact's own month**, not today's assignment.

**DECIDED: distributor is optional.** Many brands have no distributor and never will. A brand without one is modelled as the *absence of an assignment*, never a sentinel value like `Undefined`. Facts for that brand carry a null distributor, reports group them under a visible **"No distributor"**, and this never enters the review queue — it is a valid permanent state, not missing data. Users can set, change or clear an assignment at any time. Phase 0 seeded **138 real assignments**, leaving 130 brand/car-type combinations (21,394 units) deliberately unassigned.

- Today's flat lookup silently rewrites history when an agency changes hands — last year's sales get re-attributed to this year's importer. That is a real reporting bug and this fixes it.
- The migration seeds every existing assignment as valid-from the earliest data month (Jan-2021) with no end date. The business then adds the known handover dates. Until they do, behaviour is identical to today, so nothing is blocked.
- Changing an assignment's dates is a §7-flagged change: it moves published numbers and must be visible in history.

### 3.4 The Fact Table

One row per `(Governorate, Traffic Unit, Raw Brand, Raw Model, Month, Year)` → `Volume`, plus:

- The four raw strings, permanently.
- Foreign keys to the resolved car-tree and geography-tree nodes.
- A **resolution state** (see §4.3) and a link to the import batch it came from.
- Optional dimensions when the enrichment feed supplied them: license type, vehicle status, model year, fuel type.

All 13 enrichment columns are **derived at query time from the trees**, never stored on the fact. That is the fix for the `#N/A` epidemic: fixing one car-tree node instantly corrects every historical fact that points at it.

---

## 4. The AI ingestion agent

The agent's job is narrow and well-defined: **turn a raw string into a car-tree node, or admit it can't.**

### 4.1 The resolution ladder

For each incoming row, resolve brand first, then model within that brand. Try in order and stop at the first hit:

| # | Method | Confidence | Action |
|---|---|---|---|
| 1 | Exact match on a known alias | Certain | auto-link, silent |
| 2 | Normalized match — Arabic diacritics/hamza stripped, `ي/ى` and `ة/ه` unified, whitespace and case collapsed, Arabic-Indic digits converted | High | auto-link, log |
| 3 | Fuzzy match within the brand's own models above threshold | Medium | auto-link, **flag for review** |
| 4 | AI judgment — the model reasons over brand context, the full model list, and the raw string | Medium/Low | **queue for confirmation** |
| 5 | No plausible match | None | **queue as new node candidate** |

Steps 1–2 are deterministic and must run before any AI is invoked — they will handle the overwhelming majority of rows at zero cost and zero risk. The AI is for the tail. This ordering also means the system gets cheaper and faster every month as confirmed aliases accumulate.

**DECIDED: the auto-resolve thresholds are a setting, and ship conservative.**

- Two configurable numbers: the **fuzzy auto-link threshold** (tier 3) and the **AI auto-link threshold** (tier 4). Anything below them queues for review instead of linking.
- Ship them tight, so the first months over-review rather than under-review. The cost of an unnecessary confirmation is ten seconds; the cost of a silent wrong merge is a corrupted five-year trend that nobody notices.
- The settings screen should show the consequence of a change before it is saved — "at this threshold, last month's import would have auto-linked 412 more rows and queued 38 fewer" — using the stored confidence scores from prior imports. Tuning blind is how these settings get set wrong.
- Tiers 1 and 2 are **not** configurable. Exact and normalized matching are always on.

### 4.2 What the agent must never do

- **Never invent a fact.** If a row can't be resolved, the volume still lands — attached to an `Unresolved` node — so totals always reconcile against the source file. A row that silently disappears is worse than a row parked in a queue.
- **Never auto-create a brand.** New brands enter the Egyptian market a few times a year; each is a business event worth a human glance. New *models* under an existing brand may be auto-created at high confidence, because that happens monthly.
- **Never overwrite a human decision.** Once a person confirms `ZX 7 → ZX7`, the agent may not revisit it, even if a later heuristic disagrees.
- **Never resolve across brands.** `Chery Tiggo 7` and `Jetour Tiggo 7` are different cars. Brand is resolved first and constrains the model search space. This alone eliminates a large class of plausible-looking errors.

### 4.3 Resolution states

Every fact row and every tree node carries one:

- **Confirmed** — a human approved this mapping, or it matched a confirmed alias exactly.
- **Auto-resolved** — the agent linked it above the auto-threshold. Counts in reports. Reviewable but not blocking.
- **Needs review** — the agent has a proposal but isn't confident. **Counts in reports** (using the proposal) and is visibly flagged.
- **Unresolved** — no proposal. Volume is counted under `Unknown` so totals reconcile, but it appears in no brand or model breakdown.
- **Rejected** — a human said "this proposal is wrong". Feeds back as a negative example.

> Design rule: **reports never block on the review queue.** The dashboard is always usable; it just tells you honestly how much of it is provisional. A "94% confirmed / 5% auto / 1% unresolved" badge on every dashboard is more valuable than a perfect number that arrives three days late.

### 4.4 The agent also enriches, not just matches

Beyond spelling, the agent proposes attributes for genuinely new models: segment, car type, tier, engine type, origin. It has strong priors here — it knows what a BYD Seal is. These proposals should always land as **Needs review**, never auto-confirmed, because a wrong segment silently distorts every segment report thereafter.

### 4.5 Feedback loop

Every human decision in the review queue is training signal. At minimum: confirmed aliases become exact matches (tier 1) forever, and rejections become negative examples fed into the agent's prompt context for that brand. Over 3–6 months the queue should shrink to genuinely new models only.

---

## 5. View 1 — Car Data Tree

The management surface for §3. Its job is to make the trees *fast to fix*, because a person will be in here every month.

**Must have:**

- Browse and search Brand → Model → (Variant), with volume shown at every node so the user knows what matters. Fixing a node with 40,000 units is worth more than one with 3.
- Inline edit of every attribute: segment, tier, car type, engine, supply, origin, distributor.
- **The alias panel** — see every raw string that currently resolves to this node, with its volume and its source month. Detach an alias, or drag it onto another node.
- **Merge** two nodes. **Split** a node. Both with a preview of what will move and both reversible.
- **Bulk operations.** With 1,257 models and 3,779 raw brand-model strings, one-at-a-time editing is a non-starter. Multi-select → set segment. Multi-select → merge.
- **The review queue**, which is the beating heart of the view: every `Needs review` and `Unresolved` item, sorted by volume impact, with the agent's proposal, its reasoning, and one-click Confirm / Reject / Reassign. This should feel like an inbox to be emptied, not a table to be browsed.
- **Bulk confirm** for obvious cases — "all 43 proposals above 95% confidence".
- **Change history** per node: who changed what, when, and what it did to the numbers.

**DECIDED: the queue is a single shared inbox. Every user can review everything.** No per-brand ownership, no assignment, no approval chain. Simplicity is the point — the queue must be emptied, and a workflow that requires the right person to be available is a queue that grows.

The consequence is that **attribution replaces permission**: since anyone can change anything, every action records who did it (§7), and the change history is the only control. Two people working the queue at once should not silently overwrite each other — if two users open the same item, the second one to act is told the item was just resolved rather than having their decision discarded.

**Should have:**

- A **health panel** — count and volume of each resolution state, trending over time. This is how the business knows the system is working.
- **Duplicate suspects** — the agent proactively scanning the *existing* tree for nodes that look like the same car (`Mid-Bus`/`Mid Bus`, `ICE`/`ICe`). Run this once at migration and it will pay for the whole feature.
- **Impact preview before saving** — "this merge moves 12,400 units from Crossover to Compact SUV". Editing the tree silently rewrites history; the user must see that.

---

## 6. View 2 — Analytics Dashboards

The target workbook already tells us exactly what's needed. It contains five dashboards, all the same shape.

### 6.1 The universal report shape

Every dashboard in the workbook is:

> **rows = a dimension** · **columns = Jan…Dec** · **then: TTL, Share %, YTD prior year, YTD current, Growth %**

with an accompanying prior-year comparison block (`24MS%`, `25MS%`, and the delta in share points). Build this shape **once** as a configurable component and all five dashboards are the same screen with a different row dimension:

| Dashboard | Row dimension |
|---|---|
| Brand Dashboard | Brand |
| Model Dashboard | Brand-Model |
| Regions Dashboard | Region (drill → Governorate → Traffic Unit) |
| Segments Dashboard | Segment |
| Distributor Breakdown | Distributor |

Add for free, from the same component: Car Type, Engine, Origin, CKD/SUP, Tier.

### 6.2 Measures

- **Volume** — sum of units. The only base measure.
- **Share %** — of the current filter context. Note this is filter-sensitive: "MG has 7.9% share" means share of *whatever is currently selected*, and the UI must make the denominator explicit or it will be misread.
- **YTD** current vs prior year.
- **Growth %** — YoY. Guard the divide-by-zero: the source workbook is littered with `#DIV/0!` and shows `502.67` as a growth figure for a brand that sold 3 units last year. Show "new" or "n/a" instead of an absurd number.
- **Share-point delta** — change in share vs prior year. This is the metric the business actually competes on and it deserves first-class treatment, not a hidden helper column.
- **Rank** and **rank change** vs prior period.

### 6.3 Filters

Every dimension in the model is a filter, matching the workbook's slicers exactly: **Brand, Car Type, CKD/SUP, Engine, Origin, Region, Segment, Year, Month** — plus new ones the model now supports: **Distributor, Tier, Governorate, Traffic Unit**, and where enrichment feeds were ingested: **License Type, Vehicle Status, Model Year, Body Shape, Fuel Type**.

Filters must be **global and shared across all dashboards** — the workbook's biggest usability failure is that each pivot has its own disconnected slicer set. Set the filter once; every view respects it.

⚠️ **Critical default:** the workbook's Brand Dashboard total of 51,252 for Jan–Feb 2026 is *not* all vehicles — it is exactly `Passenger + Commercial + Bus`, silently excluding 58,819 motorcycles and 2,220 `Undefined`. Motorcycles are 51% of all units in the dataset. **A default filter that hides half the data must be visible, labelled, and one click from being turned off.** This is the single easiest way for the new app to be trusted more than the spreadsheet.

### 6.4 Beyond the workbook

Things the fact grain supports that Excel made impractical:

- **Time series** — the workbook only ever shows the current year in columns. Five years of monthly history is sitting in the data, unplotted.
- **Brand vs brand comparison** on one screen.
- **Distributor portfolio** — for a distributor, the mix of brands and segments they move, and where their share is shifting.
- **Segment migration** — is the market moving from Sedan to Compact SUV? The data answers this today; nothing asks it.
- **Export to the current Excel format** — do not skip this. Whoever consumes the workbook today has downstream habits built on it, and a familiar export is what buys permission to change the process.

---

## 7. The monthly workflow (what a user actually does)

This is the spine of the product. Everything above serves it.

1. **Upload** the month's file(s).
2. System **detects the file's role and grain**, extracts the period, and shows what it thinks it received. User confirms.
3. **Dry run.** Nothing is written yet. The system reports: rows read, total volume, how many rows resolve at each confidence tier, how many need review, what's new (brands, models), and — critically — **how this month's totals compare to last month's**. A 40% swing means the wrong file was uploaded.
4. **User reviews** the queue. Confirms, rejects, reassigns, creates nodes.
5. **Commit.** The month becomes live. Reports update.
6. **Reconcile** against any PDFs or aggregate reports for the same month; the system reports variances.
7. **Re-import is idempotent.** Uploading the same month again *replaces* it rather than adding to it — with a diff shown first. This will happen; the authority reissues corrected files.

Every import is a **batch** that can be inspected and rolled back whole. Given that a single bad file could corrupt five years of trend reporting, rollback is not a nice-to-have.

### 7.1 Revisions and change flagging

**DECIDED: volumes can be revised, and the model is "open editing, everything flagged" — not locked periods with an approval workflow.**

Any user can re-import a month, edit the trees, or correct a figure at any time, including for closed years. There is no sign-off gate. What the system owes in exchange is **total visibility of what changed**:

- **Every change is recorded** — who, when, what the value was before, what it is now. This covers imports, tree edits, alias moves, merges, splits, distributor re-dating, and threshold changes.
- **Changes that move published numbers are flagged**, not merely logged. A month whose totals have changed since it was first committed is marked as **revised** and shows both figures.
- **Reports surface the flag.** Any dashboard whose date range includes a revised month shows a marker, with a one-click view of what changed and when. A number that quietly differs from last week's screenshot is how people stop trusting the system.
- **Revision reasons are optional but prompted.** One free-text line when re-importing or bulk-editing a closed month. Not enforced — enforcing it just produces "update" typed a thousand times.
- **Rollback** to any prior batch stays available.

The design intent: *make the right thing easy and the surprising thing loud.* No permissions to manage, no approvals to chase, but nothing changes silently.

---

## 8. Decisions — RESOLVED

| # | Question | Decision | Detail |
|---|---|---|---|
| 1 | Tier definition | **Model-level**, separate from Segment | §3.1.2 |
| 2 | Segment map source | **Reconstruct** from the existing brand-model values, business reviews the result — ✅ done | §8.1 |
| 3 | Historical migration | **Import everything** — all 377k rows, dirt included, clean it in the app | §8.2 |
| 4 | Review queue ownership | **Single shared inbox**, all users can review anything | §5 |
| 5 | Auto-resolve threshold | **Configurable setting**, ships conservative | §4.1 |
| 6 | Distributor history | **Effective-dated from day one** | §3.3 |
| 7 | Volume revisions | **Open editing, every change flagged.** No lock, no approval chain | §7.1 |
| 8 | Motorcycles | **Excluded entirely**, dropped at ingestion, dropped volume reported | §8.3 |
| 9 | Brand casing | **All-caps consolidated** to normal case; ≤4-letter acronyms kept | §8.3 |

### 8.1 Segment map reconstruction (Phase 0)

The classification is rebuilt from the existing data rather than sourced from a file. Method:

1. Extract every distinct `(Brand, Model)` pair from `Raw Data` — distinct brand-model values — with its Segment, Car Type, Engine, CKD/SUP, and its **total volume**.
2. Normalize the known collisions: `ICE`/`ICe`/`Ice`, `HYBRID`/`Hybrid`, `Undefined`/`Undefinded`, `Mid-Bus`/`Mid Bus`, `MPV`/`MPV-B`, `HB-B`/`Car HB-B`, `SUV-3`/`Luxury SUV-3`, trailing-space brands.
3. Split `Luxury ` prefix → `Tier`.
4. Where one model carries conflicting segments across rows, resolve to the highest-volume value and **flag the conflict** for review.
5. Where segment is `Undefined` (8,494 rows), the agent proposes one; all land as Needs review.
6. **Rank the whole result by volume for review.** The business does not review 1,257 rows — it reviews the top 20, which cover the great majority of units, then works down as time allows. Everything unreviewed still functions; it is just marked as unconfirmed.

This is a data exercise, runnable immediately and in parallel with everything else. Its output — a reviewed car tree — is the seed state the application is built on.

### 8.2 Historical migration

All 377,425 rows, Jan-2021 → Feb-2026, imported as-is:

- The four raw Arabic columns become the fact's permanent identity.
- Existing enrichment columns are used to **seed the trees**, not stored on facts.
- `#N/A`, `Undefined`, `Other` and `0` values do **not** block import. They land as `Unresolved` and populate the review queue, ranked by volume — turning ~24,000 units of missing distributor and ~17,500 of missing origin into a visible, workable backlog instead of an invisible one.
- Historical months are imported as batches like any other, so they can be re-imported or rolled back individually.
- **Acceptance test:** total volume after migration = 1,107,814 units (cars only), plus 1,138,473 units reported as dropped motorcycles, together reconciling to the source workbook's 2,246,287. A migration that "cleans" totals is a broken migration — cleaning happens in the app, visibly, afterwards.

---

### 8.3 Scope rules (decisions 8 and 9)

**Motorcycles are out of scope.** A row is a motorcycle if any of `car_type = 'Motorcycle'`, `segment ∈ ('Motorcycle','Scooter')`, `engine = 'Motorcycle'`, `brand = 'Motorcycle'` — these agree on all but 215 units across five years. 148,921 rows / 1,138,473 units are dropped at ingestion.

The dropped volume must still be **counted and reported** on every import, so a month reconciles against the authority's published totals, which include motorcycles. Dropping data silently would break the variance check in §7 step 6.

Practical consequences, all good: the distributor gap collapses from 1,143,447 units to **5,943**; the review queue shrinks from 303 items to **190**; and the workbook's headline figure of 51,252 for Jan–Feb 2026 now matches the app's natural total almost exactly, because that dashboard was already excluding motorcycles behind a hidden slicer.

**Brand names are case-consolidated.** Where both spellings existed (`BAIC`/`Baic`, `SOUEAST`/`Soueast`) they merge into the normal-case form. All-caps names of 5+ letters are treated as words (`VIGOREY` → `Vigorey`). All-caps names of 4 letters or fewer are left alone, because `MG`, `BMW`, `BYD`, `DS`, `GMC`, `GAC` and `JMC` are genuine acronyms and `Bmw` would be wrong. Every change, including the acronyms held back, is listed in `phase0/seed/brand_case_changes.csv` for override.

### 8.4 Questions settled by research rather than by review

Rather than hand these to the business, they were answered from published Egyptian market sources:

| Question | Answer | Volume |
|---|---|---:|
| Hyundai `Elantra` vs `Elantra AD` vs `Elantra HD` | **One model.** Egyptian registration records carry only the Arabic nameplate; the codename split was created downstream, not by the registrar. GB Auto began local assembly of the AD in May 2024 — precisely the imported→local switch the data shows. | 29,512 |
| Hyundai `Accent` vs `Accent RB` | **One model.** Only one Accent is sold new in Egypt. | 15,192 |
| `SsangYong` vs `KGM` | **One brand.** Same legal entity, renamed March 2023. Merged, with SsangYong kept as an alias so history trends as one line. | 4,358 |
| Kia `k3` → `Cerato` or `K3`? | **K3.** The K3 sold in Egypt is the BL7 — a smaller, far cheaper car than the Cerato. The majority-vote had picked Cerato and was **wrong**. | 1,723 |
| Jetour / Exeed / Omoda / Denza / IM / Deepal / DS / Cupra / Zeekr | **Separate brands**, each with a parent link (§3.1.5). Several have a different distributor from their parent. | 26,747 |
| `Landr Cruiser` | Typo → `Land Cruiser`. | 323 |

Note the Kia case: it is the one where following the data's own majority would have produced the wrong answer. Volume is a good tie-breaker, but not a substitute for knowing what the car actually is.

## 9. Suggested phasing

**Phase 0 — Reconstruct the classification.** Per §8.1: extract brand, model, segment, tier, engine, supply, origin, and geography from the existing workbook into a clean car tree and geography tree, volume-ranked for review. *A data exercise, not a software one — it can start immediately and in parallel with everything else.*

**Phase 1 — Ingest + resolve + review.** Primary feed only. The resolution ladder, the review queue, the car tree view. Success test: **a month that today takes days of manual mapping is committed in under an hour.**

**Phase 2 — Analytics.** The universal report component and the five dashboards, global filters, Excel export. Success test: **the app reproduces the target workbook's numbers exactly**, month for month, back to 2021.

**Phase 3 — Depth.** Enrichment feeds (license type, model year, body shape, engine CC), reconciliation against PDFs, distributor effective dating, time-series and comparison views.

**Phase 4 — Leverage.** Duplicate-suspect scanning, agent learning from queue decisions, anomaly alerts ("Brand X is down 60% MoM — is that real or a mapping break?").

---

## 10. The one-sentence summary

> Build a system that ingests the traffic authority's monthly registration pivot, uses an AI agent to resolve inconsistently-spelled Arabic brand and model names against a human-owned car classification tree — flagging anything it isn't sure about rather than guessing — and then serves the resulting clean fact table through filterable dashboards that reproduce, and then exceed, the existing Excel workbook.

The dashboards are the visible product. **The car tree is the actual product.**
