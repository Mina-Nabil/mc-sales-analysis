# CLAUDE.md

Guidance for working in this repo. Read the three spec docs first:
`BUSINESS_LOGIC_PLAN.md` (why), `TECHNICAL_REQUIREMENTS.md` (build spec, the
authority for schema/API/deploy), `WEB_APP_PROPOSAL.md` (scope & cost). Phase 0
is complete; `phase0/READINESS.md` records the final decisions and canonical counts.

## What this is

A monthly Arabic car-registration ETL replacement. Raw Arabic (brand, model)
strings resolve to a human-owned Car Tree via a deterministic ladder (exact →
normalized → fuzzy) plus an AI tail on Bedrock. ~97% of rows resolve
deterministically; only ~64 genuinely-new pairs/month reach the AI.

## Non-negotiable design rules (from the specs)

- **Nothing derived is stored on a fact.** Segment, origin, distributor, region
  come from JOINs to the trees, so fixing one node re-derives all history. (TECH §2.5)
- **Every spec derives from the car tree, applied to all months by JOIN.** The
  tree is built once from the "amazing excel" (the 88MB workbook); thereafter the
  user only uploads monthly *facts* sheets, which carry **no specs at all**
  (gov/unit/brand/model/status/volume). So `car_type`, `segment`, `tier`,
  `engine_type`, `supply` are all **model-level**; `origin` is brand-level;
  `distributor` is (brand, car_type). Facts hold only raw identity + volume +
  resolved FKs (§2.5). This **reverses TECH §2.8** (which put engine/supply on
  the fact): the live feed has no engine/supply, so per-period nuance is
  impossible for new data — model-level is the only thing that applies uniformly.
  `models.engine_type`/`supply` are auto-derived (dominant historical value,
  `analytics.RefreshModelDefaults`, run after every fact load) and editable in
  the tree; editing re-derives every month's facts.
- **Model year is the one fact-level dimension.** `facts.model_year` (سنة الصنع)
  is *observed* source data, part of the row's own identity like volume — not
  derived from the tree — so §2.5 still holds. It is populated from 2026 on and
  **permanently NULL for Feb-2021 → Dec-2025**: the source workbook has no
  manufacture-year column (its `Year` col is the registration period) and no raw
  archive survives for those months. 'Unknown' is the honest value, not a guess.
- **Model aliases are scoped to a brand.** Never resolve a model alias globally. (TECH §2.2)
- **Distributor is optional** — modelled as absence of a row, never a sentinel.
  NULL distributor is a valid permanent state, never enters the review queue. (TECH §2.4)
- **Motorcycles excluded by config flag**, but always counted & reported so totals
  reconcile against authority figures. (TECH §0.1, §11.6)
- **Brand/model casing:** names ≤4 letters → ALL CAPS; longer all-caps → Title case. (TECH §0.1)
- **Migration must reconcile exactly** — never "clean" totals during import. (TECH §8.1)
- **Every mutation writes `change_log`** (human | agent | migration). (TECH §2.6)

## Local dev

`docker compose up -d` then `./bin/server migrate && ./bin/server seed`.
DB is Postgres 16 on host port **5433**; `DATABASE_URL` defaults to it.

## Seed notes (learned while building the loader)

- Seed CSVs carry a UTF-8 BOM on the header line — the loader strips it.
- Brands reference their parent by name (`parent_brand`); loaded in two passes.
- Alias CSVs give a `norm_key` + ` | `-separated `raw_spellings`; one alias row is
  inserted per raw spelling (so alias row counts exceed the seed-row counts:
  ~332 brand + ~2644 model alias rows from 315 + 2510 seed rows).
- `distributor_assignments.csv` uses two non-canonical brand names: `Ssangyong`
  (merged → KGM, per `research_decisions.csv`) and `LI` (→ `Li Auto`). The loader
  canonicalizes both and skips the resulting merge-duplicate assignment, so the DB
  holds **137** assignments from the 138 seed rows.
- Segments: 24 seeded, **13 in use** by history.

## Fact migration notes (`migrate-facts`, TECH §8)

- The "amazing excel" (`260308_*.xlsx`) lives in `dump/`; `migrate-facts` looks
  there then repo root, or honours `SOURCE_WORKBOOK`. It backfills each model's
  `engine_type`/`supply` default (via `RefreshModelDefaults`) after loading, so
  the tree carries the specs the monthly feeds lack.
- Reads the "Raw Data" sheet (20 cols, layout in BUSINESS §1.1). Store raw
  identity **untrimmed, numeric coerced to string** — that is what reproduces the
  §8.1 distinct-raw counts (3,779 pairs / 781 brands / 222 units, car rows only).
- Motorcycle detection uses the workbook's own derived columns (car type,
  segment, engine, brand). Dropped volume is recorded per batch, not discarded.
- **Deviation from TECH §2.5:** the documented per-batch UNIQUE natural key was
  relaxed to a non-unique index. The key is unique from 2026 on (17,600/17,600)
  but the 2021–2024 source has **808 duplicate (gov,unit,brand,model,period)
  groups / 5,061 rows** that differ in no raw or fact-level column. Faithful
  migration must keep all 377,425 rows (§8.1), so uniqueness is asserted by query
  where it holds, not by constraint. Duplicates sum correctly in analytics.
- The whole import runs in one transaction (all-or-nothing); re-running is
  guarded by a `facts` non-empty check. To redo: `docker compose down -v`.

## Live monthly import (`import` / `import-commit`, TECH §4)

- **Preferred feed `brands_models_by_year`** — <span dir="rtl">إحصائية الماركات
  والطرازات للمركبات الزيرو</span>, "zero vehicles by year of manufacture". Same
  first five columns as the older feed, then a <span dir="rtl">سنة الصنع</span>
  block whose 4-digit year sub-columns are read from **row 3 dynamically** (the
  authority slides the window: Jan/Feb-2026 report 2022–2026, Mar onward
  2023–2027). One source row unpivots into one fact per non-empty year cell.
  Proven an **exact** decomposition of the old feed's `Zero` column — same keys,
  same values, 0 discrepancies across all six overlapping months
  (`TestFeedsReconcileExactly`). It is also ~5× fewer fact rows, because the
  by-status feed stores every row that appeared under *any* status and ~85% of
  those carry volume 0.
- **Decoy guard:** every archive also holds <span dir="rtl">تقرير … المركبات
  الملاكي …</span> — private plates only, *identical* header signature, ~35% of
  the units (Jul-2026: 22,378 vs 63,219). `rejectPrivatePlateVariant` matches the
  **normalized** title (these sheets carry tatweel throughout, so a raw substring
  test is unreliable).
- **Residual guard:** if a row's year cells do not sum to its own grand total,
  the difference is emitted as one `model_year = NULL` fact, so the period still
  reconciles exactly (§8.1) instead of silently losing units. Never observed in
  seven months, but it makes the property structural rather than lucky.
- Fallback feed `brands_models_by_status`: title row carries the period
  (`من YYYY/MM/DD …`), row 2 = dimension labels, row 3 = status sub-columns.
  **Volume = the `Zero` column**, and `model_year` is left NULL — the dry run
  says so before you commit. Governorate/unit are forward-filled; subtotal
  rows (blank brand+model) are skipped.
- `scripts/extract-feeds.py [dump] [out] --kind year|status` pulls whichever
  shape you want out of the archives (`feeds-year/` and `feeds/` respectively).
  It probes every xlsx in an archive, since several share a header shape.
- **Live motorcycle exclusion:** the monthly feed has no Car Type column, so §0.1
  can't fire at parse time. `motorcycle_keys` (seeded, migration 0002) carries the
  no-space normalized brand / brand-model keys history proved to be motorcycles;
  the importer drops + counts them when `ingest.exclude_motorcycles=true`. Built
  from the source workbook (139 pure brands + 280 brand-model keys). Validated on
  Jul-2026: drops 8,012 rows / 34,894 units, leaving 63,743 car rows / 28,325
  units (+6.7% vs Feb, sane — vs +138% if motorcycles were left in).
- Re-committing a period **supersedes** the prior batch (old → `rolled_back`,
  facts deleted and re-inserted) — idempotent revision (§4.4).
- **Full history load:** `migrate-facts` (workbook, Feb-2021→Feb-2026) then
  `load-feeds feeds/` for the later months. Raw monthly archives live in `dump/`
  (`fwd*.zip`, non-UTF8 Arabic names); `scripts/extract-feeds.py` pulls just the
  primary feed from each → `feeds/YYYY-MM.xlsx`. `load-feeds` commits them in
  period order and **skips already-committed periods** unless `--revise`, so it's
  safe to re-run. Both `dump/` and `feeds/` are git-ignored (local source data).
- Overlap check (Aug 2026 drop): the new feeds' Jan/Feb/Jul 2026 match the
  workbook (Feb & Jul exact, Jan within 1 unit). **March 2026 is missing** from
  the delivered files, so 2026 has months 01,02,04,05,06,07. Current DB span:
  Feb-2021 → Jul-2026, 64 periods, 628,014 facts / 1,216,143 units.
- Seed fix: the Phase 0 CSVs HTML-over-escaped `Lynk & Co` as `Lynk &amp; Co`
  (20 rows). The seed loader now `html.UnescapeString`s every cell, so names,
  aliases and normalized keys agree and `LYNK&CO` from the feed resolves.

## Fuzzy (tier 3) + review queue (`internal/domain/fuzzy.go`, `internal/review/`)

- `FuzzyScore` = max(normalized-Levenshtein, token-SORT ratio). Deliberately
  token-**sort**, not token-set: token-set returns 1.0 for a subset like
  "x70" ⊂ "x70 plus" — the exact merge §3.3 forbids.
- Two guards gate auto-linking (`CanAutoLink`): **DigitsDiffer** (X70/X90,
  Tiggo 7/8) and **VariantDiffer** (plus/pro/max/fl/ev/gt/… token differs). A
  guarded pair can still be *proposed* for review, never auto-linked.
- `review.Resolve` runs fuzzy over distinct (brand, raw_model) where brand is
  resolved but model isn't: ≥threshold & guards-clear → `auto_resolved`;
  ≥floor → `needs_review` with a proposal; else → `needs_review`, no proposal
  (new-model candidate). Review items ARE `model_aliases` rows with
  status='needs_review' (no separate table); volume is joined from facts.
- **Four decisions per row** (all offered on every item, not just when a proposal
  exists): *merge into a model you pick* (`Confirm`/`reassign` + the Merge modal,
  brand-scoped list, proposal preselected), *new model* (`CreateModelForReview`),
  *wrong match* (`Reject` — proposal was wrong, volume returns to `unresolved`
  and the rejected model is kept in `reasoning` as the negative example, §4.5),
  and *not needed* (`Exclude` — volume is out of scope). Keys J/K/Enter/M/N/R/X.
- **`Exclude` marks, never deletes**: facts → `status='rejected'`, and analytics
  filters them out via the `notExcluded` predicate in `Matrix`/`Values`/
  `Aggregate`/`Timeseries` (the first fact-status filter in that layer). Rows stay
  countable so period totals still reconcile — §0.1 "counted & reported", §8.1
  "never clean totals". Excluded units are returned by the endpoint and shown in
  the modal; a reported per-period line is still to do.
- All decisions write `change_log` with units in `volume_impact`. Optimistic
  concurrency: a decide fails if status ≠ needs_review.
- **`Resolve` skips pairs with a `rejected` alias.** It used to re-select them
  (`model_id IS NULL AND status='unresolved'`) and re-run the unconditional facts
  UPDATE, silently re-linking the very model a reviewer had rejected — while
  `upsertAlias`'s ON CONFLICT guard left the alias `rejected`, so `List` never
  showed the item again. Fixed with a `NOT EXISTS` guard.
- **`ingest.ReresolveUnresolved` replays tiers 1/2/2b over stored facts** after
  every import and every review decision (`server reresolve`, `--dry-run` to
  preview). Confirming an alias only re-derives that exact raw spelling; ~5% of
  normalized model keys have more than one spelling, so siblings already in
  `facts` would otherwise wait for a fuzzy pass. The API returns what the replay
  settled as `replay` on each decision response.
- **Fixed: `tree.CreateModel` had 6 value expressions for 5 columns** (the same
  defect as the seed loader's, commit cb322d4), so *every* "new model" decision —
  review queue and Tree page — failed at the database. Regression test in
  `internal/tree/tree_test.go`.
- Brand-unresolved facts (no brand_id) are surfaced in a separate **brand queue**
  (`tree.BrandQueue`) and resolved via `tree.ResolveBrand` (create/assign a brand
  → alias raw→brand → set brand_id on facts, which then flow to the model queue).
  Tier-4 AI still pending.

## Tree editing + node creation (`internal/tree/`, TECH §6.3, §4.2)

- `CreateBrand`/`CreateModel` apply the §0.1 casing rule (`applyCasing`: ≤4
  letters → ALL CAPS; longer all-caps → Title case; else as typed).
- `EditModel` just updates the node — segment/tier/car_type are join-derived onto
  facts, so history re-derives automatically; the affected unit count is still
  logged to `change_log` for the impact trail.
- `MergeModels` (same brand only) re-points aliases (skipping raw collisions) and
  facts to the survivor, sets `merged_into`, marks the source `rejected`.
  `MergePreview` powers the pre-merge impact dialog (§6.3).
- `CreateModelForReview` closes a "new model" queue item: create the model +
  confirm the alias + re-derive its facts, in one call.
- Every mutation writes `change_log` (actor_kind='human', units in volume_impact).
- SPA: Review page has Models/New-brands tabs with new-model + brand-resolve
  modals; Tree page has New brand/New model buttons, an inline model editor,
  merge dialog, and alias detach. Split is still not built.

## HTTP API + auth (`internal/api/`, `internal/auth/`, TECH §7 / §11.5)

- One binary, `server serve` on `$PORT` (default 8080). Router is stdlib
  `http.ServeMux` with method+pattern routes (Go 1.22+). The React build is
  embedded via `assets.go` (`web/dist`, placeholder page for now) and served as
  the SPA fallback; JSON API under `/api/v1`.
- Auth: email+password, **argon2id** (PHC-encoded), server-side `sessions` with
  an HttpOnly/SameSite=Lax cookie (`mc_session`). `SECURE_COOKIES=true` behind
  TLS. No roles — every mutation is attributed in `change_log`.
- `seed-admin` reads `ADMIN_EMAIL`/`ADMIN_PASSWORD`; refuses to run if other
  users exist unless `--force` (idempotent update of the same admin).
- Imports over HTTP: `POST /imports` (multipart) saves the file under
  `$UPLOAD_DIR` (default `./uploads`, git-ignored) keyed by a random token, then
  `GET /imports/{token}/dry-run` and `POST /imports/{token}/commit` re-parse it.
  `DetectAndParse`/`DryRun`/`Commit` are reused verbatim from the CLI path.
- Full analytics matrix (§5.1) is still Phase 2; `/analytics/confidence` and
  `/stats` are the only aggregates exposed so far.

## Front-end (`web/`, React 19 + TS + Tailwind v4)

- Built on the **Vela** admin template (licensed ThemeForest, kept out of git in
  `theme-template/`). Its `components/ui`, `components/charts` (dependency-free
  SVG), `lib`, `theme` and `index.css` (design tokens, dark/light) were copied
  into `web/src`; our own `layout/`, `pages/`, `lib/api.ts`, `auth.tsx` wire them
  to the Go API. `@/*` path alias → `web/src`.
- `npm run build` → `web/dist`, embedded via `assets.go`, served by `server serve`
  (`spaHandler` deep-link fallback). **`web/dist` is committed** so `go build`
  works without a JS toolchain; rebuild dist then the Go binary after FE changes.
  **In production the SPA is served from S3 + CloudFront** (edge redesign, TECH §11);
  the embed stays for local/dev and as a fallback. CloudFront routes `/api/*` to the
  Fargate task (via a t4g.nano Caddy proxy) and `/*` to the S3 SPA bucket, so a FE
  deploy is `npm run build` → `aws s3 sync web/dist` → CloudFront invalidation, with
  no Go rebuild.
- Pages: Overview, Analytics (charts), Dashboard (§5.1 matrix + Excel export),
  Review (J/K/Enter/R/N keys, new-brand/model modals), Tree (edit/merge/detach),
  Import, Settings, Audit. Raw Arabic uses `dir="rtl"`/`dir="auto"`.
- Browser-testing notes: React controlled inputs need the `form_input` tool; the
  stale `vite.config.js` from the first SPA once shadowed `.ts` — only one config.
- **npm optional-deps bug:** Tailwind/rollup/lightningcss native binaries may not
  install; add the platform pkg (Intel mac: `@tailwindcss/oxide-darwin-x64`,
  `@rollup/rollup-darwin-x64`, `lightningcss-darwin-x64`). Doesn't affect `go build`.

## Analytics matrix (`internal/analytics/`, TECH §5.1)

- One parameterized query powers every dashboard: `Matrix(dimension, year,
  compare_year, filters, months, limit)`. Measure = SUM(volume); dimensions are
  JOIN-derived with COALESCE→'Unknown'/'No distributor' so shares reconcile.
- Growth % is **YTD-based** (same month window both years) so a partial current
  year compares fairly. Shares/ranks/share-point-delta computed in Go from the
  grouped rows. Distributor dim uses an effective-dated LATERAL join.
- `model_year` and `model_age` (`period_year - model_year`) are the only
  fact-level dimensions; both COALESCE to 'Unknown' and are sorted numerically
  (Unknown last) rather than by volume. Excluded facts (`status='rejected'`) are
  filtered out of `Matrix`/`Values`/`Aggregate`/`Timeseries` by `notExcluded`.
- API: `GET /analytics/matrix`, `/analytics/dimensions`, `/analytics/export.xlsx`
  (excelize, workbook column layout). Frontend `Dashboard.tsx`: dimension/year/
  filter selectors, top-N bar chart, the full month matrix, export button.

## Conventions

- Go 1.26, module `github.com/Mina-Nabil/mc-sales-analysis`. One binary, subcommands.
- Migrations are plain SQL in `/migrations`, embedded via `assets.go`, applied by
  `internal/store`. Add the next as `0002_*.sql`.
- Postgres access via `pgx/v5` / `pgxpool`. Analytics will be hand-written SQL (no ORM).
- No secrets in the repo. AWS via named profile; AI via Bedrock IAM role (no API key).
