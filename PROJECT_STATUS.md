# Project Status & Next Steps

**Last updated:** 2026-08-20 · **Head commit:** `8ec3350` · **Branch:** `master`

A session-handoff for picking the project up cold. For *why* the system works
the way it does, read `BUSINESS_LOGIC_PLAN.md`, `TECHNICAL_REQUIREMENTS.md`,
`WEB_APP_PROPOSAL.md`; for build conventions read `CLAUDE.md`; to run it read
`README.md`. This file is the "where we are / what's next" layer on top.

---

## TL;DR

Phases **0, 1 and 2 are done, committed, and verified**. The app is a working
web product (auth → import → resolve → review → tree editing → analytics matrix
+ dashboards + Excel export + global filters) and is **containerized and
deploy-ready**. The next major piece is **Infrastructure-as-Code (Terraform,
Option A−)**, which needs a few inputs from you (below). Tier-4 AI (Bedrock) is
optional and can land after deploy.

---

## How to boot locally (cold start)

Requires Go 1.26+ and Docker. Postgres runs on host port **5433**.

```bash
docker compose up -d
export DATABASE_URL="postgres://mc:mc@localhost:5433/mcsales?sslmode=disable"
go build -o bin/server ./cmd/server

./bin/server migrate            # schema (also auto-runs on `serve`)
./bin/server seed               # Phase 0 classification (embedded CSVs)
ADMIN_EMAIL=admin@motorcity.local ADMIN_PASSWORD=devpassword123 ./bin/server seed-admin

# full fact history (needs the source Excel — see "Data" below):
./bin/server migrate-facts      # amazing excel (dump/), 2021→Feb-2026, runs §8.1 tests
./bin/server load-feeds feeds/  # monthly feeds → adds Apr/May/Jun/Jul 2026
./bin/server resolve            # tier-3 fuzzy pass over the backlog

PORT=8090 ./bin/server serve    # → http://localhost:8090  (login with the admin above)
```

Front-end changes: `cd web && npm run build` then rebuild the Go binary (the SPA
is embedded). Container: `docker build -t mc-sales .` (18 MB distroless image).

---

## What's built (done ✅)

**Phase 0 — classification** (pre-existing): 201 brands · 1,243 models · 13
segments in use · 2,825 aliases, all in `seed/*.csv`, loaded by `server seed`.

**Phase 1 — ingest → resolve → review → tree**
- Migrations `0001–0004`; schema per TECH §2 (car tree, aliases, geo, effective-
  dated distributors, facts, batches, change_log, settings, users/sessions).
- Normalization (§3.2) + resolution ladder tiers 1–2b (exact/normalized/no-space)
  with table-driven tests. Historical migration (`migrate-facts`) passes all §8.1
  acceptance tests.
- Live monthly import: signature detection, forward-fill parser, dry-run, commit,
  idempotent revision; motorcycle exclusion via `motorcycle_keys`.
- Tier-3 **fuzzy** matching with digit + variant guards; **review queue** ops
  (list/confirm/reject/reassign/bulk-confirm) that re-derive facts.
- **Tree editing + node creation**: create brand/model (incl. from the review
  queue), edit model attributes, merge, alias detach; brand-resolution queue.
- HTTP API + argon2id auth + sessions; React 19 + Tailwind v4 SPA (Vela template),
  dark/light, embedded in the binary.

**Phase 2 — analytics**
- One parameterized **matrix** query (`/analytics/matrix`): any dimension × month,
  share %, YTD/period growth, share-point delta, rank + movement. Compares any two
  periods (year-over-year, month-over-month, mirror-month).
- **Dashboard** (matrix + top-N chart + Excel export) and **Analytics** (trend /
  bar / donut) pages, routed through filter-aware `agg`/`timeseries` endpoints.
- **Global multi-select filters** encoded in the URL (shareable), persistent
  across pages; `.../export.xlsx` in the workbook column layout.

**Deploy readiness**
- Multi-stage **Dockerfile** (distroless, CGO-free, ARM64-ready via buildx), SPA +
  migrations + seed embedded; `serve` auto-migrates on startup. Smoke-tested.

---

## Key design decisions & deviations (don't re-litigate)

- **All car specs derive from the car tree, applied to every month by JOIN.**
  `car_type`/`segment`/`tier`/`engine_type`/`supply` are **model-level**; `origin`
  is brand-level; `distributor` is (brand, car_type). Facts hold only raw identity
  + volume + resolved FKs. This **reverses TECH §2.8** (which put engine/supply on
  the fact): the monthly feeds carry no specs, so model-level is the only thing
  that applies to new data. `models.engine_type`/`supply` are auto-derived
  (`analytics.RefreshModelDefaults`, run after every fact load) and editable in the
  tree. — commit `2085968`.
- **Facts natural-key UNIQUE (§2.5) relaxed to a non-unique index**: the 2021–2024
  source has 808 duplicate (gov,unit,brand,model,period) groups that must be kept
  to reconcile §8.1. Unique from 2026 on; asserted by query where it holds.
- **Model-level engine/supply** deliberately loses per-period nuance the amazing
  excel had (a model that switched ICE→HYBRID); accepted, because new months have
  no engine data at all.

---

## Data state

- **Span:** Feb-2021 → Jul-2026, **64 committed periods**, ~628k facts / 1.22M car
  units. 2026 has months **01,02,04,05,06,07 — March is missing** from the source
  files (the one gap; analytics handle it).
- **Source files (git-ignored, local):** `dump/` holds the raw archives — the
  "amazing excel" (`260308_*.xlsx`, full history + all specs) and the monthly
  `fwd*.zip` bundles. `scripts/extract-feeds.py` pulls just the one primary feed
  (`إحصائية الماركات والطرازات وفقاً لحالة المركبة.xlsx`) from each zip into
  `feeds/YYYY-MM.xlsx`. Overlap verified: new feeds match the workbook per-brand
  (Feb & Jul exact, Jan within 1 unit).
- **Each new month:** the user uploads that one primary feed via the Import page
  (or `load-feeds`), nothing else in the bundle.

---

## Upcoming steps (next session, prioritized)

### 1. Infrastructure-as-Code — Terraform, Option A− (the immediate next)
Target (decided in `phase0/READINESS.md` §2 / TECH §11): AWS `eu-central-1`, ECS
Fargate (1 task, 0.5 vCPU/1 GB, ARM64), RDS `db.t4g.small` Single-AZ, **Cloudflare
Tunnel (no ALB)**, S3, Bedrock via IAM role + VPC endpoint. **No** NAT gateway,
**no** ALB, **no** Multi-AZ (§11.2). Scaffold `/infra`:
- state backend (S3 + DynamoDB lock) — bootstrap first (§11.1)
- VPC + public subnets (tasks in public subnet, tight SG; VPC endpoints for S3 +
  Bedrock so no NAT)
- ECR repo + RDS + Secrets/SSM for `DATABASE_URL`
- ECS cluster/service/task (image = this Dockerfile; `serve` auto-migrates)
- Cloudflare Tunnel (cloudflared as a sidecar or separate task) → the app on :8080
- billing alarm at $115/mo (§11.1)

**Inputs needed from you** (operator-specific; some are secrets — set on the
machine, don't paste here):
- AWS **named profile** + account ID (region `eu-central-1` already fixed)
- Cloudflare **hostname/domain** + a **tunnel token** (created in your CF account)
- Confirm **Bedrock Claude model access** is enabled in the account
- Whether to include a **CI workflow** (GitHub Actions → buildx arm64 → ECR) now

### 2. Production data bootstrap
Seed CSVs are embedded (work anywhere), but the ~628k facts need the source Excel
loaded once in prod. Plan: a one-off ECS task running `migrate-facts` + `load-feeds`
with the files pulled from S3, **or** load them through the Import UI. Decide which.

### 3. Tier-4 AI (Bedrock) — optional, post-deploy
Wire `internal/ai` (currently absent): Bedrock client (EU inference profile,
Claude Sonnet, IAM role — no key), prompt/schema per TECH §3.4, batch + prompt
caching, feeds proposals into the review queue. App is ~97% deterministic without
it. Needs Bedrock access enabled.

### 4. Smaller items / backlog
- **March 2026**: drop the file in `dump/`, re-run `extract-feeds.py` + `load-feeds`.
- Backups (§9): RDS automated + weekly logical dump of the **tree tables** to S3.
- S3 for uploads/exports (Fargate fs is ephemeral; single-task upload→commit is
  fine today).
- Model **split** (§6.3), distributor effective-dating UI, SSE import progress,
  in-app user management, revision markers on dashboards (§6.4).
- Enrichment feeds (license type, model year, body shape) — Phase 3 (§4.5).

---

## Repo map

```
/cmd/server        one binary; subcommands (see below)
/internal/domain   normalization + fuzzy (pure, tested)
/internal/ingest   parsers, signature detection, migrate, dry-run, commit, resolver
/internal/review   fuzzy resolve pass + review-queue ops
/internal/tree     brand/model create/edit/merge, brand queue
/internal/analytics matrix, agg, timeseries, values, RefreshModelDefaults
/internal/api      REST + auth middleware + embedded SPA
/internal/auth     argon2id + sessions
/internal/store    pgx pool + migration runner
/migrations        0001–0004 (embedded)
/seed              15 Phase 0 CSVs (embedded; the seed state)
/web               React 19 + TS + Tailwind v4; built to web/dist (committed, embedded)
/scripts           extract-feeds.py
/infra             EMPTY — Terraform goes here next
Dockerfile         distroless, ARM64-ready
```

**Subcommands:** `migrate · seed · seed-admin · migrate-facts · import ·
import-commit · load-feeds · refresh-model-defaults · resolve · review · serve`.

---

## Recent commits

```
8ec3350 deploy: containerize (distroless, ARM64-ready) + auto-migrate on serve
2085968 specs: derive ALL car specs from the tree, applied to every month
48e278e data: combine full fact history Feb-2021 → Jul-2026
a849f95 analytics: global multi-select filters, shareable URLs, tooltip fix
e523b5b analytics: single-period matrix layout, header tooltips, year filter
e8ebb21 analytics: compare any two periods (month-over-month, not just years)
6e775e8 phase2: analytics matrix (§5.1) + Excel export + dashboard UI
833710d phase1: done
```
