# Egypt Car Sales Analytics — Web App Proposal

**Prepared for:** Motorcity
**Date:** August 2026
**Companion documents:** `BUSINESS_LOGIC_PLAN.md` (business logic), `TECHNICAL_REQUIREMENTS.md` (build spec)

---

## 1. The problem, in one number

Every month, someone receives a registration report from the Egyptian traffic authority containing roughly **8,000 rows** of car sales data. Every row has an Arabic brand name and an Arabic model name, spelled however the issuing traffic unit happened to spell it that month.

Before that data can be used, each row must be mapped to a clean classification: which brand is this really, which model, what segment, what engine type, is it locally assembled or imported, who distributes it. Today that mapping is done **by hand, in Excel, every month.**

We analysed five years of that work — 377,425 rows covering 1,107,814 vehicles from February 2021 to February 2026 — and measured how much of it is genuinely new:

> Of the ~8,000 rows arriving each month, an average of **64** contain a brand/model combination that has never been seen before.
>
> **99.2% of the monthly effort is re-doing work that was already done.**

That is the entire business case. The remaining 0.8% is real judgement that a human should make. Everything else is repetition, and repetition is what software is for.

**This is now proven, not projected.** We built the classification (Phase 0, complete). Replayed against five years of history, it resolves **98.0% of rows automatically with no AI involvement at all** — purely by recognising spellings it has already been taught.

*Scope note: motorcycles are excluded throughout, per your instruction — 1,138,473 units removed from a raw file of 2,246,287. All figures in this document are cars, buses, commercial and construction vehicles only.*

## 2. What the current process costs in accuracy

The manual process does not merely take time — it produces errors that nothing in Excel will ever flag. Measured across the 377,425 car rows:

| Defect in the current data | Rows | Vehicles | % of volume |
|---|---:|---:|---:|
| Distributor recorded as `Undefined` | 15,887 | 24,130 | 2.2% |
| Distributor lookup returned an error (`#N/A`) | 10,225 | 23,497 | 2.1% |
| Model recorded as `Undefined` or `Other` | 10,296 | 20,594 | 1.9% |
| Car type missing | 8,479 | 17,663 | 1.6% |
| Segment missing or misspelled | 8,478 | 17,665 | 1.6% |
| Country of origin missing (fell through to `0`) | 8,531 | 17,547 | 1.6% |
| Brand recorded as `Undefined` | 8,414 | 17,381 | 1.6% |
| Local-assembly vs import flag missing | 7,374 | 15,604 | 1.4% |
| Region blank or a broken formula | 5,284 | 12,230 | 1.1% |

**We want to be straight about this**, because an earlier draft of this proposal overstated it. Before motorcycles were excluded, the distributor error affected 1,160,394 vehicles and we described it as *"half the market cannot be attributed to a distributor."* That was true of the raw file, but the cause was mundane: the distributor lookup was built for cars and never had motorcycle entries. **For cars, the figure is 2.1%.**

The stronger argument is not that the data is broken — it is mostly fine. It is that:

- **Silent inconsistency corrupts every grouped report.** The engine field contains `ICE`, `ICe` and `Ice` as three separate values. The segment field had `Mid-Bus` and `Mid Bus`, `Undefined` and `Undefinded`. Brands split their own volume across `BAIC` and `Baic`, `SOUEAST` and `Soueast`. Every one of these quietly understates a total, and nothing tells you.
- **The classification exists nowhere as a file.** No formula, no lookup sheet — 3,779 brand-model values pasted from human memory. Your most valuable analytical asset is unowned, unversioned and unbacked-up.
- **Fixing anything means redoing the month.** There is no way to correct one mapping and have five years of history re-derive.

## 3. What we propose to build

A web application with two surfaces.

### 3.1 The Car Data Tree — the asset

A managed, versioned classification of every brand, model and variant sold in Egypt: **body-type segment** (Sedan, Hatchback, Crossover, Compact SUV, SUV, MPV, Van, Pickup …), tier (highline/baseline), car type, engine type, local-assembly status, country of origin, and — optionally — distributor.

Critically, each node owns its **aliases** — every raw Arabic spelling that means this car. Once someone confirms that `ZX 7` and `ZX7` are the same car, that decision holds forever, for every past month and every future one. The tree is where institutional knowledge finally lives somewhere other than in one person's head.

### 3.2 The Analytics Dashboards — the product

Everything the current workbook shows, and then everything it cannot: five years of monthly history, brand and model rankings with share and growth, regional breakdowns down to individual traffic units, and segment, tier and distributor analysis. One global filter set that every view respects — unlike today, where each pivot has its own disconnected slicers.

### 3.3 The AI agent — the bridge between them

When a month is uploaded, the system resolves each row against the tree in stages: exact match first, then normalised match (Arabic diacritics, `ي/ى`, `ة/ه`, spacing, digits), then fuzzy match, and only then AI judgement. The deterministic stages are free and handle the overwhelming majority. The AI handles the tail — and when it isn't confident, **it says so rather than guessing.**

Anything uncertain lands in a review queue, ranked by volume so the biggest questions come first. Nothing is ever silently dropped: unresolved rows still count toward totals under `Unknown`, so the numbers always reconcile against the source file.

## 4. What changes for the user

| | Today | With the app |
|---|---|---|
| Monthly mapping | ~8,000 rows by hand | ~64 decisions in a review queue |
| Fixing a past mistake | Re-do the month, or live with it | Edit one tree node; all history re-derives |
| "Which distributor?" | Flat lookup — an agency change silently rewrites history | Effective-dated, so past years stay true |
| Trend over 5 years | Not built — the workbook shows one year in columns | Standard view |
| "Share of highline in Compact SUV?" | Impossible — highline and baseline used different size scales | Answerable; both share one body-type segment |
| Trust | No way to know what's wrong | Every dashboard shows what % is confirmed |
| Knowledge | In one person's head | In a versioned, auditable system |

The target for Phase 1 is blunt and measurable: **a month that today takes days should be committed in under an hour.**

---

## 5. Cost

All figures are **US East (N. Virginia) on-demand list prices, August 2026**. Frankfurt, Bahrain and UAE regions run roughly **10–20% higher**; confirm final numbers in the AWS Pricing Calculator once a region is chosen. Prices exclude VAT and any Enterprise Discount Program terms.

### 5.1 Sizing note — this is a small system

It is worth being explicit, because it is easy to be sold far more infrastructure than this needs:

- **377,425 rows today**, growing ~96,000 rows/year. After ten years the fact table is under 1.4 million rows.
- Estimated database size including indexes and full audit history: **under 5 GB**.
- Usage is a handful of internal analysts, not public traffic.
- The heaviest operation — importing and resolving a month — runs a few times a month and takes minutes.

This is a small-data problem wearing a big-data costume — and excluding motorcycles made it 50% smaller again. A single modest database instance and one small container genuinely suffice, and the selected option below reflects that.

### 5.2 AWS — five options

The cost driver here is **fixed always-on infrastructure serving a workload that is idle ~99% of the time.** A handful of analysts run queries during working hours; the heavy job runs for a few minutes once a month. Every option below serves that workload adequately — they differ in how much operational convenience you buy.

| | Option | us-east-1 | Frankfurt / Gulf | vs. baseline | |
|---|---|---:|---:|---:|---|
| **A** | Managed, as first proposed | $120 | $138 | — | |
| **A−** | **Same architecture, right-sized** | **$49** | **$56** | **−60%** | ✅ **SELECTED** |
| **A− +** | A− with 1-year commitments | $39 | $45 | −68% | *later, once measured* |
| **L2** | Lightsail: instance + managed Postgres | $29 | $33 | −76% | |
| **L1** | Lightsail: single box, self-hosted Postgres | $16 | $18 | −87% | |
| **B** | Production high-availability | $292 | $336 | +143% | *only when load-bearing* |

**Decision: Option A− at ~$56/month (Frankfurt), ~$57 all-in with AI.** Lightsail would have saved a further $38/month, but standard ECS + RDS primitives produce more reliable agent-generated infrastructure-as-code and are easier to hand to a different engineer later. That difference is worth $38/month.

---

**Option A− — right-sized managed AWS (SELECTED)**

| Component | Spec | Monthly |
|---|---|---:|
| ECS Fargate — API + worker in one task | 1 × 0.5 vCPU / 1 GB, 24/7 | $18.02 |
| RDS PostgreSQL | **db.t4g.small** (2 vCPU / 2 GB), Single-AZ | $23.36 |
| Storage | 20 GB gp3 | $2.30 |
| Load balancer | **none** — TLS terminated in-app on a static IP, or via Cloudflare Tunnel (free) | $0 |
| S3 (uploads, exports, backups) | | ~$2 |
| CloudWatch (7-day retention), Secrets, ECR | | ~$3 |
| **Total (us-east-1)** | | **~$49** |
| **Frankfurt / Bahrain / UAE (+15%)** | | **~$56** |

What changed from Option A, and why each is safe:

- **db.t4g.medium → db.t4g.small, −$24.** The database is under 5 GB. At 2 GB RAM the entire fact table sits in cache. `t4g.medium` was headroom for a problem this system does not have.
- **Dropped the Application Load Balancer, −$21.** An ALB costs $16.43/month just to exist. For a few internal users it buys nothing a Go binary serving TLS directly cannot do. Cloudflare Tunnel is free and additionally removes the need for any public ingress at all.
- **Two Fargate tasks → one, −$18.** The import worker runs a few minutes a month. Running it as a second 24/7 container is paying rent on an empty room; run it in-process in the API binary (the spec already builds them as one binary, two modes).
- **50 GB → 20 GB storage, −$3.** Ten years of facts plus full audit history fits in single-digit gigabytes.
- **CloudWatch retention set to 7 days, −$4.** The default is *never expire*, which silently grows forever.

**Option A− + commitments — $39/month.** A 1-year no-upfront RDS Reserved Instance takes `db.t4g.small` from $23.36 to $17.01, and a Compute Savings Plan takes ~20% off Fargate. Worth doing **after** a few months of real usage, not before — commit to a size you have measured, not one you guessed.

**Options L1 / L2 — Lightsail.** Lightsail is flat-rate AWS with data transfer included. A `Small-2GB` instance is $12/month with 60 GB SSD and 3 TB transfer. Either run Postgres on the same box (**L1, ~$16/month all-in**) or pair it with a Lightsail managed database (**L2, ~$29/month**, automated backups included). Both genuinely serve this workload.

The trade-off is not performance — it is **standardness**. Lightsail has a smaller surface area, fewer knobs, and less well-trodden infrastructure-as-code. Since this project will be built by coding agents, ECS + RDS will produce more reliable generated infrastructure and is easier to hand to a different engineer later. *Not selected for this reason, but a legitimate fallback if the monthly figure ever needs to come down further.*

**Option B — high availability, $292.** Multi-AZ RDS doubles the database cost, NAT Gateways add $65.70, and a second availability zone doubles compute. Buy this when the tool is load-bearing enough that an afternoon of downtime costs real money — not before.

### 5.3 Cost traps to avoid

Each of these is a default that quietly costs more than the entire right-sized stack:

| Trap | Cost | Avoid by |
|---|---:|---|
| **NAT Gateway** | $32.85/mo **per AZ** + data processing | Put tasks in public subnets with tight security groups, or use VPC endpoints. Two AZs of NAT costs more than all of Option A− combined. |
| **Multi-AZ RDS before it's needed** | +$23–95/mo | Single-AZ with automated backups until downtime has a measured cost. |
| **ALB for a handful of users** | $16.43/mo minimum | In-app TLS, or Cloudflare Tunnel (free). |
| **CloudWatch default retention** | grows forever | Set 7–30 day retention on day one. |
| **Aurora Serverless v2** | min 0.5 ACU ≈ $43/mo | It does not scale to zero. It is *more* expensive than `db.t4g.small` here. |
| **Over-provisioned "just in case"** | 2–3× | The measured working set is under 5 GB. Size to that and scale later — scaling up is a 10-minute operation. |

If this is a new AWS account, the 12-month Free Tier covers `db.t4g.micro` and much of the rest — useful for the pilot, but do not let it drive architecture, because it expires.

### 5.4 AI agent — running cost

This is the number most likely to be over-estimated, so here is the arithmetic.

The AI is **not** called per row. It is called only for distinct brand/model strings that the deterministic stages could not resolve. From the five years of data we measured, that is on the order of **64 genuinely new pairs per month**, plus low-confidence fuzzy matches — budget **~200 AI decisions per month**.

Assumptions: ~3,000 input tokens per decision (system prompt, the brand's model catalogue, candidate context), ~250 output tokens (a structured decision with reasoning). Prompt caching applies because the catalogue and taxonomy are identical across calls. Batch processing applies because imports are not latency-sensitive.

**Steady-state monthly cost:**

| Model | List | + prompt caching | + caching & batch |
|---|---:|---:|---:|
| Claude Haiku 4.5 | $0.85 | $0.47 | **$0.24** |
| Claude Sonnet 5 | $1.70 | $0.94 | **$0.47** |
| Claude Opus 5 | $4.25 | $2.36 | **$1.18** |

Even under a deliberately pessimistic scenario — **4× the call volume and 33% longer prompts** — Sonnet 5 costs $9.60/month and Opus 5 costs $24/month at full list price.

**One-time migration cost** (classifying all 3,779 historical brand/model pairs with richer prompts covering segment, tier, engine and origin) — *note: Phase 0 has already been completed deterministically, so this now applies only to the residual tail:*

| Model | List | + caching & batch |
|---|---:|---:|
| Claude Haiku 4.5 | $22.67 | **$6.24** |
| Claude Sonnet 5 | $45.35 | **$12.47** |
| Claude Opus 5 | $113.37 | **$31.18** |

**Recommendation: Claude Sonnet 5 for resolution, at roughly $0.50–2/month.** Haiku 4.5 is a reasonable fallback if volume grows unexpectedly, but the cost difference is a rounding error against the value of a correct decision, and this is precisely the kind of judgement task where the better model earns its keep. Reserve Opus 5 for the one-time migration pass, where accuracy compounds across all future months and the total spend is under $32.

> **The AI is not the expensive part.** Infrastructure costs 40–100× more than the intelligence running on it. Any proposal that treats the AI as the cost driver here has the arithmetic backwards.

### 5.5 Total running cost

All figures Frankfurt (add ~15% to us-east-1 list). AI is Sonnet 5 with prompt caching and the Batch API.

| Scenario | AWS | AI | **Total / month** |
|---|---:|---:|---:|
| **A− Right-sized managed** ✅ **SELECTED** | ~$56 | ~$0.50 | **~$57** |
| A− with 1-year commitments | ~$45 | ~$1 | **~$46** |
| L2 Lightsail + managed Postgres | ~$33 | ~$1 | **~$34** |
| L1 Lightsail single box | ~$18 | ~$1 | **~$19** |
| B Production high-availability | ~$336 | ~$1 | **~$337** |

Plus a **one-time** AI migration cost of **$12–31**.

Two things worth noting about this table. First, the recommended configuration costs less per month than a single analyst-hour — against a process that currently consumes days of one every month. Second, at $57/month the AI is under 1% of the bill: **the intelligence is free and the infrastructure is the cost**, which is the opposite of what most people assume when they hear "AI agent".

### 5.6 Build effort

Engineering effort only — excludes internal review time and your own labour rates.

| Phase | Scope | Estimate |
|---|---|---|
| **0** | Reconstruct the classification from existing data (a data exercise, runs in parallel) | 1–2 weeks |
| **1** | Ingestion, resolution engine, review queue, car tree view | 5–7 weeks |
| **2** | Analytics dashboards, global filters, Excel export | 4–5 weeks |
| **3** | Enrichment feeds, reconciliation, map, distributor dating, time series | 4–6 weeks |
| **4** | Duplicate scanning, agent learning, anomaly alerts | 2–3 weeks |

**Phases 0–2 deliver a system that fully replaces the current workbook: roughly 10–14 weeks.** Phases 3–4 are additive and can be scheduled against observed need rather than committed up front.

---

## 6. Scope boundaries

**In scope:** ingesting the traffic authority's monthly registration reports; the classification tree and its review workflow; the analytics dashboards listed in §3.2; Excel export in the current format; full change history.

**Explicitly excluded by decision:**

- **Motorcycles** — 1,138,473 units, 50.7% of the raw file — dropped at ingestion, with dropped volume still reported on every import so totals reconcile against the authority's published figures.
- **Any map view**, and therefore any storage of coordinates.
- **Segment size codes** — segments are body types (Sedan, Crossover, Compact SUV, SUV …); vehicle size is not tracked, and premium-ness is carried by tier instead.

**Out of scope for now,** and worth naming so it isn't assumed: pricing or revenue data (the source contains unit volumes only — there is no price field anywhere in the data); dealer-level or VIN-level detail (the source is pre-aggregated); forecasting; and any write-back to the traffic authority.

**Deprioritised:** PDF ingestion. The five PDFs in the sample are watermarked, right-to-left, and their extracted text is visually rather than logically ordered — the highest effort and lowest value input in the set. They are useful for reconciliation (checking that a month's total matches the authority's published figure) and should be treated as such, not as a data source.

---

## 7. Risks and how we handle them

| Risk | Mitigation |
|---|---|
| **The AI mis-maps a car and it goes unnoticed** | Conservative confidence thresholds ship by default; every auto-resolution is logged and reviewable; dashboards display what proportion of the data is confirmed vs auto-resolved. The system is designed to be visibly uncertain rather than confidently wrong. |
| **The source file format changes** | The importer detects the file's role and grain from its header signature and refuses to proceed on an unrecognised shape rather than mis-parsing it. A dry-run precedes every commit. |
| **A bad import corrupts history** | Every import is a batch that can be rolled back whole. Re-importing a month replaces it rather than adding to it, with a diff shown first. |
| **The reconstructed classification is wrong** | It is volume-ranked for review: the top ~200 models cover the great majority of units. Unreviewed entries still function, marked as unconfirmed. |
| **Key-person dependency persists** | This is precisely what the tree eliminates — but it only works if the review queue is actually worked. Success should be measured on queue size trending toward zero, not just on the app existing. |
| **AWS costs drift** | The architecture is deliberately small. Set a billing alarm at 2× the expected figure on day one; at this scale anything above that indicates a design fault, not growth. The traps in §5.3 — NAT Gateways, premature Multi-AZ, unbounded log retention — are how a $57 bill silently becomes $200. |

---

## 8. Recommendation

Proceed with **Phase 0 immediately** — reconstructing the classification is a data exercise that requires no software and de-risks everything after it. Build **Phases 1 and 2** on the right-sized **Option A−** configuration at roughly **$57/month all-in**, add Reserved Instances once a few months of real usage have been measured, and revisit high availability only once the tool is genuinely load-bearing for the business.

The dashboards are what people will ask for. **The car tree is what they actually need**, and it is the part that stops being someone's spreadsheet and starts being a company asset.

---

*Pricing sources: [Claude Platform pricing](https://platform.claude.com/docs/en/about-claude/pricing), [AWS RDS db.t4g.medium](https://instances.vantage.sh/aws/rds/db.t4g.medium), [AWS Fargate rates](https://fortem.dev/blog/aws-fargate-pricing-real-costs/), [AWS ALB pricing](https://www.cloudzero.com/blog/aws-alb-pricing/). All verified August 2026.*
