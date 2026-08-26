# VPC with public subnets (Fargate task + proxy egress via IGW — no NAT) and
# private subnets (RDS only). VPC endpoints for S3 + Bedrock keep those off the
# internet (§11.2, §11.4). Cloud Map private namespace gives the task a stable
# DNS name the proxy can resolve despite the ephemeral task IP.

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs      = slice(data.aws_availability_zones.available.names, 0, 2)
  vpc_cidr = "10.20.0.0/16"
  name     = "mc-sales-${var.env}"
  # /20 public + /20 private per AZ
  public_cidrs  = ["10.20.0.0/20", "10.20.16.0/20"]
  private_cidrs = ["10.20.128.0/20", "10.20.144.0/20"]
}

resource "aws_vpc" "this" {
  cidr_block           = local.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags                 = { Name = local.name }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = local.name }
}

resource "aws_subnet" "public" {
  count                   = 2
  vpc_id                  = aws_vpc.this.id
  cidr_block              = local.public_cidrs[count.index]
  availability_zone       = local.azs[count.index]
  map_public_ip_on_launch = true
  tags                    = { Name = "${local.name}-public-${count.index}", Tier = "public" }
}

resource "aws_subnet" "private" {
  count             = 2
  vpc_id            = aws_vpc.this.id
  cidr_block        = local.private_cidrs[count.index]
  availability_zone = local.azs[count.index]
  tags              = { Name = "${local.name}-private-${count.index}", Tier = "private" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }
  tags = { Name = "${local.name}-public" }
}

resource "aws_route_table_association" "public" {
  count          = 2
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Private subnets have NO default route (no NAT). Only the S3 gateway endpoint
# and the interface endpoints reach out. RDS needs no internet.
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${local.name}-private" }
}

resource "aws_route_table_association" "private" {
  count          = 2
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# ── Security groups ─────────────────────────────────────────────────────────
# NOTE: a rule referencing a managed prefix list counts against the per-SG rule
# limit (default 60) as (max entries in the list) — the CloudFront list is ~55.
# So each SG may hold only ONE such rule. Ingress is split into separate,
# count-gated rules (not inline blocks) so the proxy vs. direct paths don't both
# consume prefix-list rules on the same group.

data "aws_ec2_managed_prefix_list" "cloudfront" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

# Proxy SG — only built when the nano proxy is in play. Caddy listens on :80
# only (TLS is at CloudFront), so a single prefix-list rule suffices.
resource "aws_security_group" "proxy" {
  count       = var.use_proxy ? 1 : 0
  name        = "${local.name}-proxy"
  description = "CloudFront to Caddy proxy"
  vpc_id      = aws_vpc.this.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "${local.name}-proxy" }
}

resource "aws_security_group_rule" "proxy_http_from_cf" {
  count             = var.use_proxy ? 1 : 0
  type              = "ingress"
  security_group_id = aws_security_group.proxy[0].id
  from_port         = 80
  to_port           = 80
  protocol          = "tcp"
  prefix_list_ids   = [data.aws_ec2_managed_prefix_list.cloudfront.id]
  description       = "HTTP from CloudFront edge only"
}

# Task SG — egress open (ECR/CW/Bedrock via IGW). Ingress on 8080 comes from the
# proxy SG (use_proxy) OR directly from the CloudFront prefix list (direct).
resource "aws_security_group" "task" {
  name        = "${local.name}-task"
  description = "Fargate app task"
  vpc_id      = aws_vpc.this.id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
  tags = { Name = "${local.name}-task" }
}

resource "aws_security_group_rule" "task_from_proxy" {
  count                    = var.use_proxy ? 1 : 0
  type                     = "ingress"
  security_group_id        = aws_security_group.task.id
  from_port                = 8080
  to_port                  = 8080
  protocol                 = "tcp"
  source_security_group_id = aws_security_group.proxy[0].id
  description              = "App port from proxy only"
}

# use_proxy=false: CloudFront hits the task directly (one prefix-list rule).
resource "aws_security_group_rule" "task_from_cloudfront" {
  count             = var.use_proxy ? 0 : 1
  type              = "ingress"
  security_group_id = aws_security_group.task.id
  from_port         = 8080
  to_port           = 8080
  protocol          = "tcp"
  prefix_list_ids   = [data.aws_ec2_managed_prefix_list.cloudfront.id]
  description       = "Direct CloudFront origin"
}

# DB: only the task SG may reach 5432.
resource "aws_security_group" "db" {
  name        = "${local.name}-db"
  description = "RDS Postgres"
  vpc_id      = aws_vpc.this.id

  ingress {
    description     = "Postgres from task"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.task.id]
  }
  tags = { Name = "${local.name}-db" }
}

# Interface endpoints (Bedrock) accept HTTPS from the task SG.
resource "aws_security_group" "endpoints" {
  name        = "${local.name}-vpce"
  description = "VPC interface endpoints"
  vpc_id      = aws_vpc.this.id

  ingress {
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.task.id]
  }
  tags = { Name = "${local.name}-vpce" }
}

# ── VPC endpoints (§11.4: no NAT) ───────────────────────────────────────────
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.public.id, aws_route_table.private.id]
  tags              = { Name = "${local.name}-s3" }
}

resource "aws_vpc_endpoint" "bedrock" {
  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.${var.region}.bedrock-runtime"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.public[*].id
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = true
  tags                = { Name = "${local.name}-bedrock" }
}

# ── Cloud Map (service discovery for the ephemeral task) ─────────────────────
resource "aws_service_discovery_private_dns_namespace" "this" {
  name        = "mc.local"
  vpc         = aws_vpc.this.id
  description = "Private DNS for the app task; proxy resolves api.mc.local"
}
