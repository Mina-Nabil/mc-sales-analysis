# Infrastructure (Terraform)

Option A− for the Motorcity sales-analytics app, **`eu-central-1` (Frankfurt)**,
edge redesign of 2026-08-23 (CloudFront + S3 SPA + t4g.nano Caddy proxy, no
Cloudflare, no ALB, no NAT). See `TECHNICAL_REQUIREMENTS.md` §11 for the why.

```
CloudFront ──┬─ "/*"     → S3 (private, OAC)         = React SPA (web/dist)
             └─ "/api/*" → t4g.nano Caddy proxy (EIP) → Fargate task (Cloud Map)
                                                          → RDS db.t4g.small
```

## Layout

| Dir | What | State |
|---|---|---|
| `bootstrap/` | S3 state bucket + DynamoDB lock table | **local** (run once, first) |
| root (`.`) | provider + backend + wires the modules below | S3 (after bootstrap) |
| `modules/network` | VPC, public subnets, IGW, SGs, VPC endpoints, Cloud Map | |
| `modules/data` | RDS Postgres + SSM/Secrets for `DATABASE_URL` | |
| `modules/storage` | app S3 bucket (uploads/exports/dumps, IA at 90d) | |
| `modules/compute` | ECR, ECS cluster/service/task, IAM roles, Cloud Map service | |
| `modules/edge-proxy` | t4g.nano EC2 + EIP + Caddy (only when `use_proxy=true`) | |
| `modules/dns-updater` | Lambda+EventBridge → Route53 (only when `use_proxy=false`) | |
| `modules/edge-cdn` | S3 SPA bucket, ACM (us-east-1), CloudFront, Route53 | |
| `modules/observability` | SNS + $115/mo billing alarm (us-east-1 provider) | |

## Prerequisites (operator inputs — never commit secrets)

- A **named AWS profile** with admin-ish rights (default `mc_profile`, see `.env`).
- A registered **domain** and a **Route53 public hosted zone** for it.
- **Bedrock Claude model access** enabled in the account (for Tier-4 AI, later).
- The **admin bootstrap** password for the app (`ADMIN_PASSWORD`) — set via
  `TF_VAR_admin_password`, not in a committed `.tfvars`.

## Apply order

```bash
# 0. one-time state backend
cd bootstrap
terraform init && terraform apply
# copy the bucket + table names it outputs into ../backend.hcl

# 1. everything else
cd ..
cp terraform.tfvars.example terraform.tfvars   # fill in domain, hosted zone, etc.
terraform init -backend-config=backend.hcl
export TF_VAR_admin_password='…'               # not in tfvars
terraform apply
```

`terraform plan` must show **no** `aws_lb`, `aws_nat_gateway`, or Multi-AZ RDS —
those are the cost traps (§11.2). The `$115/mo` billing alarm is created day one.

## Notes

- ACM certs for CloudFront **must** live in `us-east-1`; the root config declares a
  second provider alias (`aws.us_east_1`) for exactly that. Everything else is
  `eu-central-1`.
- The Fargate task keeps a **public IP in a public subnet** for egress (ECR /
  CloudWatch / Bedrock via the IGW); its security group only admits the proxy, so
  it is not publicly reachable. VPC endpoints (S3 gateway + Bedrock interface)
  keep those two off the internet.
- **Ingress is chosen by `use_proxy`** (currently **`false`** in `terraform.tfvars`):
  - `true` → the `edge-proxy` module builds a t4g.nano Caddy box (EIP) as the
    CloudFront `/api` origin on :80.
  - `false` → the `dns-updater` module: a Lambda fired by EventBridge on each ECS
    task reaching RUNNING writes the task's **public** IP into a Route53 record
    (`api.<domain>`), which CloudFront uses as the `/api` origin on :8080. No box,
    ~$7/mo cheaper. Tradeoffs: a brief 5xx window during task recycles while
    Route53 + CloudFront re-resolve, and — like the proxy path — a plain-HTTP
    CloudFront→origin hop (only an ALB+ACM cert at ~$16/mo can encrypt it).
    Needed because ECS service discovery only publishes the task's *private* IP,
    which the CloudFront edge cannot reach.
```
