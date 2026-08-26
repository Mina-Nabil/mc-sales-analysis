# ECR + one ECS Fargate service (ARM64) running the app image. `serve`
# auto-migrates on boot (§8 / Dockerfile CMD). Registered in Cloud Map as
# api.mc.local so the proxy has a stable name for the ephemeral task.

data "aws_region" "current" {}
data "aws_caller_identity" "current" {}

locals {
  name      = "mc-sales-${var.env}"
  log_group = "/ecs/${local.name}"

  # EU cross-region inference profile for Claude (§11.4) — specific ARN, not "*".
  bedrock_profile_arn = "arn:aws:bedrock:${var.region}:${data.aws_caller_identity.current.account_id}:inference-profile/eu.anthropic.claude-*"
}

resource "aws_ecr_repository" "app" {
  name                 = local.name
  image_tag_mutability = "MUTABLE"
  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "app" {
  repository = aws_ecr_repository.app.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "keep last 10 images"
      selection    = { tagStatus = "any", countType = "imageCountMoreThan", countNumber = 10 }
      action       = { type = "expire" }
    }]
  })
}

resource "aws_cloudwatch_log_group" "app" {
  name              = local.log_group
  retention_in_days = 7 # §11
}

resource "aws_ecs_cluster" "this" {
  name = local.name
  setting {
    name  = "containerInsights"
    value = "disabled" # keeps CloudWatch cost down
  }
}

# ── IAM ─────────────────────────────────────────────────────────────────────
data "aws_iam_policy_document" "assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Execution role: pull image, write logs, read SSM params for the container env.
resource "aws_iam_role" "execution" {
  name               = "${local.name}-exec"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_ssm" {
  name = "read-ssm"
  role = aws_iam_role.execution.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["ssm:GetParameters"]
      Resource = [var.database_url_ssm_arn, var.admin_password_ssm]
    }]
  })
}

# Task role: what the app itself may call — Bedrock (specific profile) + app S3.
resource "aws_iam_role" "task" {
  name               = "${local.name}-task"
  assume_role_policy = data.aws_iam_policy_document.assume.json
}

resource "aws_iam_role_policy" "task_bedrock" {
  name = "bedrock-invoke"
  role = aws_iam_role.task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["bedrock:InvokeModel", "bedrock:InvokeModelWithResponseStream"]
      Resource = local.bedrock_profile_arn # §11.4 — specific profile, not "*"
    }]
  })
}

resource "aws_iam_role_policy" "task_s3" {
  name = "app-bucket"
  role = aws_iam_role.task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["s3:GetObject", "s3:PutObject", "s3:ListBucket", "s3:DeleteObject"]
      Resource = [var.app_bucket_arn, "${var.app_bucket_arn}/*"]
    }]
  })
}

# ── Cloud Map service ───────────────────────────────────────────────────────
resource "aws_service_discovery_service" "api" {
  name = "api"
  dns_config {
    namespace_id = var.cloud_map_namespace
    dns_records {
      type = "A"
      ttl  = 10 # short so proxy re-resolves quickly on task recycle
    }
    routing_policy = "MULTIVALUE"
  }
  health_check_custom_config {
    failure_threshold = 1
  }
}

# ── Task definition + service ───────────────────────────────────────────────
resource "aws_ecs_task_definition" "app" {
  family                   = local.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  runtime_platform {
    cpu_architecture        = "ARM64" # §11.7 Graviton
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([{
    name         = "app"
    image        = "${aws_ecr_repository.app.repository_url}:${var.image_tag}"
    essential    = true
    command      = ["serve"]
    portMappings = [{ containerPort = 8080, protocol = "tcp" }]
    environment = [
      { name = "PORT", value = "8080" },
      { name = "SECURE_COOKIES", value = "true" },
      { name = "ADMIN_EMAIL", value = var.admin_email },
      { name = "AWS_REGION", value = var.region },
    ]
    secrets = [
      { name = "DATABASE_URL", valueFrom = var.database_url_ssm_arn },
      { name = "ADMIN_PASSWORD", valueFrom = var.admin_password_ssm },
    ]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.app.name
        "awslogs-region"        = data.aws_region.current.name
        "awslogs-stream-prefix" = "app"
      }
    }
  }])
}

resource "aws_ecs_service" "app" {
  name            = local.name
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.subnet_ids
    security_groups  = [var.task_security_group]
    assign_public_ip = true # public subnet + IGW = egress without NAT
  }

  service_registries {
    registry_arn = aws_service_discovery_service.api.arn
  }

  # avoid churn when CI pushes a new image with the same "latest" tag path
  lifecycle {
    ignore_changes = [task_definition]
  }
}
