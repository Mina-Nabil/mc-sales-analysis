# mc-sales-analysis

Egypt Car Sales Analytics for Motorcity. Ingests the traffic authority's monthly
registration pivots, resolves inconsistently-spelled Arabic brand/model names
against a human-owned classification tree, and serves clean facts through
filterable dashboards.

See `BUSINESS_LOGIC_PLAN.md` (why), `TECHNICAL_REQUIREMENTS.md` (build spec),
and `WEB_APP_PROPOSAL.md` (scope & cost). Phase 0 (the classification) is done;
its output is the 15 CSVs in `seed/`.

## Stack

Go (one binary) · React (embedded) · PostgreSQL · AWS (Fargate + RDS, Bedrock).

## Local development

Requires Go 1.26+ and Docker.

```bash
# 1. start Postgres (host port 5433)
docker compose up -d

# 2. build the binary
go build -o bin/server ./cmd/server

# 3. apply the schema, then load the Phase 0 classification
export DATABASE_URL="postgres://mc:mc@localhost:5433/mcsales?sslmode=disable"
./bin/server migrate
./bin/server seed

# 4. (optional) load the full fact history, Feb-2021 → Jul-2026.
#    a) the 2021→Feb-2026 workbook — the "amazing excel", kept in dump/ (or set
#       SOURCE_WORKBOOK). Runs the §8.1 acceptance tests and derives each model's
#       engine/supply spec from it:
./bin/server migrate-facts
#    b) the later monthly feeds. Extract the primary feed from each raw archive
#       in dump/ (see scripts/extract-feeds.py), then load them (idempotent —
#       already-committed periods are skipped unless --revise):
python3 scripts/extract-feeds.py          # dump/*.zip → feeds/YYYY-MM.xlsx
./bin/server load-feeds feeds/            # adds Apr/May/Jun/Jul 2026
./bin/server resolve                      # fuzzy pass over the new backlog
```

> Data note: the monthly feeds overlap the workbook on Jan/Feb 2026 and match it
> (Feb exact; Jan within 1 unit) — the overlap is a consistency check, and
> `load-feeds` keeps the existing batch. **March 2026 is absent from the source
> files**, so the series has that one gap; the analytics handle missing months.

`server seed` is guarded: it refuses to run if the `brands` table is non-empty.
To reset from scratch:

```bash
docker compose down -v && docker compose up -d
```

## Commands

| Command | Status | Purpose |
|---|---|---|
| `server migrate` | ✅ | apply `migrations/*.sql` |
| `server seed` | ✅ | load the Phase 0 classification from `seed/*.csv` |
| `server migrate-facts` | ✅ | one-shot historical migration + §8.1 acceptance tests |
| `server import <file>` | ✅ | detect + parse + **dry-run** a monthly feed (no writes) |
| `server import-commit <file> [reason]` | ✅ | commit a monthly feed as a batch (idempotent revision) |
| `server load-feeds <dir> [--revise]` | ✅ | commit every primary feed in a dir, in period order (skips already-committed) |
| `server refresh-model-defaults` | ✅ | recompute model engine/supply defaults from facts (auto-run after loads) |
| `server resolve` | ✅ | tier-3 fuzzy pass over the unresolved backlog → auto-links + proposals |
| `server review list [n]` | ✅ | the review queue, ranked by volume impact |
| `server review confirm <aliasID> [modelID]` | ✅ | accept an item; re-derives all its facts |
| `server review reassign <aliasID> <modelID>` | ✅ | confirm to a different model |
| `server review reject <aliasID>` | ✅ | reject a proposal (kept as a negative example) |
| `server review bulk-confirm <minConfidence>` | ✅ | confirm all proposals at/above a confidence |
| `server seed-admin` | ✅ | create/update the first admin user (from `ADMIN_EMAIL`/`ADMIN_PASSWORD`) |
| `server serve` | ✅ | run the HTTP API (+ embedded SPA) on `$PORT` (default 8080) |

## HTTP API

```bash
# create the first user, then run the server
export DATABASE_URL="postgres://mc:mc@localhost:5433/mcsales?sslmode=disable"
ADMIN_EMAIL=admin@motorcity.local ADMIN_PASSWORD=change-me-8+chars ./bin/server seed-admin
./bin/server serve            # http://localhost:8080
```

Auth is email+password (argon2id) with an HttpOnly session cookie; no roles —
every action is attributed in `change_log`. Endpoints under `/api/v1` (TECH §7):

| Area | Endpoints |
|---|---|
| auth | `POST /auth/login` · `POST /auth/logout` · `GET /auth/me` · `POST /users` |
| review | `GET /review` · `POST /review/{id}/confirm\|reject\|reassign` · `POST /review/bulk-confirm` · `POST /resolve` |
| tree (read) | `GET /brands` · `GET /brands/{id}/models` · `GET /models/{id}/aliases` · `GET /segments` · `GET /distributors` |
| tree (edit) | `POST /brands` · `POST /models` · `PATCH /models/{id}` · `GET /models/{id}/merge-preview` · `POST /models/{id}/merge` · `DELETE /aliases/{id}` |
| brand/model creation | `GET /review/brands` · `POST /review/brands/resolve` · `POST /review/{id}/new-model` |
| imports | `POST /imports` (multipart) · `GET /imports/{id}/dry-run` · `POST /imports/{id}/commit` · `GET /imports` |
| misc | `GET/PATCH /settings` · `GET /changes` · `GET /analytics/confidence` · `GET /stats` · `GET /healthz` |

```bash
# example: log in and read the review queue
curl -c j.txt -X POST localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"Email":"admin@motorcity.local","Password":"change-me-8+chars"}'
curl -b j.txt "localhost:8080/api/v1/review?limit=20"
```

Set `SECURE_COOKIES=true` behind TLS (Cloudflare Tunnel). Uploads are stored
under `$UPLOAD_DIR` (default `./uploads`).

## Front-end (React SPA)

A **Vite + React 19 + TypeScript + Tailwind v4** app in `web/`, built to
`web/dist/` and **embedded in the Go binary** — `server serve` hosts it at `/`
with deep-link fallback. Dark/light themed. Screens: Overview, **Analytics**
(trend/bar/donut charts), **Dashboard** (the §5.1 matrix with Excel export),
Review queue (keyboard inbox + new-brand/model), Car tree (browse + edit +
merge), Import (upload → dry-run → commit), Settings, Audit.

```bash
cd web
npm install
npm run build          # → web/dist (committed; the Go build embeds it)
npm run dev            # optional: Vite dev server on :5173, proxies /api to :8080
```

After front-end changes, re-run `npm run build` then rebuild the Go binary.

> **npm optional-deps caveat:** on some npm versions the Tailwind/rollup native
> binaries fail to install. If `npm run build` errors with "Cannot find native
> binding", install the matching platform packages, e.g. on Intel macOS:
> `npm i @tailwindcss/oxide-darwin-x64 @rollup/rollup-darwin-x64 lightningcss-darwin-x64`.

The analytics matrix (§5.1) is served by `GET /api/v1/analytics/matrix`
(dimension × months, share/growth/rank) with `.../export.xlsx` for the workbook
format.

Resolution ladder: exact → normalized → no-space (tiers 1–2b, at import) → fuzzy
(tier 3, `resolve`) → AI (tier 4, not yet). Fuzzy has a **digit guard** (X70≠X90,
Tiggo 7≠Tiggo 8) and a **variant guard** (x70≠x70 Plus) so those never auto-link —
they route to review. Confirming an item writes a `confirmed` alias (tier-1
forever) and re-derives every fact for that raw string; every action is written to
`change_log`. Concurrent edits are caught (the second actor is told it was already
decided).

Importing a real month (the Jul-2026 primary feed in `mainsource/`):

```bash
./bin/server import "mainsource/.إحصائية الماركات والطرازات وفقاً لحالة المركبة.xlsx"
```

The dry-run reports the detected signature/period, motorcycles dropped (still
counted), resolution by tier, new brands/models ranked by volume, and the
month-over-month variance (flagged if >30%). `import-commit` writes the facts;
re-committing the same period supersedes the prior batch rather than duplicating.

All §8.1 acceptance tests pass, and the deterministic resolver reproduces the
Phase 0 economics independently: **98.03% brand / 96.79% brand+model** resolved
with no fuzzy matching and no AI.

## Layout

```
/cmd/server        main.go (CLI: migrate | seed | serve | seed-admin)
/internal/store    DB connection + migration runner
/internal/seed     Phase 0 seed loader
/internal/domain   resolution engine (pure, no I/O)   — not yet built
/internal/ingest   parsers, signature detection       — not yet built
/internal/analytics the matrix query                  — not yet built
/internal/ai       Bedrock client                     — not yet built
/internal/api      REST handlers                       — not yet built
/migrations        golang-style SQL migrations (embedded)
/seed              the 15 Phase 0 CSVs (embedded — the seed state)
/web               React app                           — not yet built
/infra             Terraform                           — not yet built
```
