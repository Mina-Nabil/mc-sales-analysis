# RDS Postgres 16, db.t4g.small, Single-AZ, 7-day backups, gp3 autoscaling
# (§11). Password generated and stored in SSM; DATABASE_URL assembled for the app.

locals {
  name = "mc-sales-${var.env}"
}

resource "aws_db_subnet_group" "this" {
  name       = local.name
  subnet_ids = var.subnet_ids
}

resource "random_password" "db" {
  length  = 32
  special = false # keep the URL clean; 32 alnum chars is plenty
}

resource "aws_db_parameter_group" "this" {
  name   = local.name
  family = "postgres16"
}

resource "aws_db_instance" "this" {
  identifier     = local.name
  engine         = "postgres"
  engine_version = "16"
  instance_class = var.instance_class

  allocated_storage     = var.allocated_storage
  max_allocated_storage = var.max_allocated_storage
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = "mcsales"
  username = "mc"
  password = random_password.db.result

  multi_az               = false # §11.2 — Single-AZ until downtime has a measured cost
  publicly_accessible    = false
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [var.db_security_group_id]
  parameter_group_name   = aws_db_parameter_group.this.name

  backup_retention_period   = 7
  backup_window             = "02:00-03:00"
  maintenance_window        = "Mon:03:30-Mon:04:30"
  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "${local.name}-final"
  apply_immediately         = false
}

# ── App secrets in SSM Parameter Store (free; §11.2 keeps Secrets Manager out) ──
resource "aws_ssm_parameter" "database_url" {
  name  = "/${local.name}/DATABASE_URL"
  type  = "SecureString"
  value = "postgres://mc:${random_password.db.result}@${aws_db_instance.this.address}:5432/mcsales?sslmode=require"
}

resource "aws_ssm_parameter" "admin_password" {
  name  = "/${local.name}/ADMIN_PASSWORD"
  type  = "SecureString"
  value = var.admin_password
}
