# use_proxy=false path: keep the Fargate task as the direct CloudFront origin,
# with no ALB and no nano proxy. ECS service discovery only publishes the task's
# PRIVATE IP, which CloudFront (public internet) cannot reach — so a small Lambda
# fired by EventBridge on each task launch writes the task's PUBLIC IP into a
# Route53 A record. CloudFront's /api origin points at that record.
#
# Tradeoffs vs. the proxy (documented in TECH §11):
#   - a brief 5xx window during task recycles until Route53 + CloudFront re-resolve;
#   - CloudFront -> origin is plain HTTP over the public internet (same as the
#     proxy path). Viewer -> CloudFront is TLS. An ALB with an ACM cert is the
#     only way to encrypt the origin hop, at ~$16/mo.

terraform {
  required_providers {
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.4"
    }
  }
}

locals {
  record_fqdn = "${var.subdomain}.${var.domain_name}"
}

# ── Route53 record (seeded, then owned by the Lambda) ───────────────────────
resource "aws_route53_record" "api" {
  zone_id = var.route53_zone_id
  name    = local.record_fqdn
  type    = "A"
  ttl     = var.ttl
  records = ["192.0.2.1"] # TEST-NET placeholder; Lambda UPSERTs the real IP

  lifecycle {
    ignore_changes = [records] # the Lambda is the source of truth after apply
  }
}

# ── Lambda ──────────────────────────────────────────────────────────────────
data "archive_file" "lambda" {
  type        = "zip"
  source_file = "${path.module}/lambda/handler.py"
  output_path = "${path.module}/lambda/handler.zip"
}

data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name               = "mc-sales-dnsupdater-${var.env}"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

resource "aws_iam_role_policy" "lambda" {
  name = "dns-updater"
  role = aws_iam_role.lambda.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["ec2:DescribeNetworkInterfaces"]
        Resource = "*" # Describe* does not support resource-level scoping
      },
      {
        Effect   = "Allow"
        Action   = ["route53:ChangeResourceRecordSets"]
        Resource = "arn:aws:route53:::hostedzone/${var.route53_zone_id}"
      },
      {
        Effect   = "Allow"
        Action   = ["logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"]
        Resource = "arn:aws:logs:*:*:*"
      },
    ]
  })
}

resource "aws_lambda_function" "updater" {
  function_name    = "mc-sales-dnsupdater-${var.env}"
  role             = aws_iam_role.lambda.arn
  runtime          = "python3.12"
  handler          = "handler.handler"
  filename         = data.archive_file.lambda.output_path
  source_code_hash = data.archive_file.lambda.output_base64sha256
  timeout          = 15

  environment {
    variables = {
      ZONE_ID     = var.route53_zone_id
      RECORD_NAME = local.record_fqdn
      TTL         = tostring(var.ttl)
    }
  }
}

resource "aws_cloudwatch_log_group" "lambda" {
  name              = "/aws/lambda/${aws_lambda_function.updater.function_name}"
  retention_in_days = 7
}

# ── EventBridge: fire on our cluster's tasks reaching RUNNING ────────────────
resource "aws_cloudwatch_event_rule" "task_running" {
  name        = "mc-sales-task-running-${var.env}"
  description = "ECS task RUNNING triggers API origin DNS refresh"
  event_pattern = jsonencode({
    source      = ["aws.ecs"]
    detail-type = ["ECS Task State Change"]
    detail = {
      clusterArn    = [var.cluster_arn]
      lastStatus    = ["RUNNING"]
      desiredStatus = ["RUNNING"]
    }
  })
}

resource "aws_cloudwatch_event_target" "lambda" {
  rule      = aws_cloudwatch_event_rule.task_running.name
  target_id = "dns-updater"
  arn       = aws_lambda_function.updater.arn
}

resource "aws_lambda_permission" "events" {
  statement_id  = "AllowEventBridge"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.updater.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.task_running.arn
}
