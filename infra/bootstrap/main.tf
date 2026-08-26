# Terraform state backend — run this ONCE, before the root config.
# Uses a LOCAL state file (committed to nothing) to create the S3 bucket +
# DynamoDB lock table that the root config then uses as its remote backend.

terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }
}

provider "aws" {
  region  = var.region
  profile = var.profile
}

variable "region" {
  type    = string
  default = "eu-central-1"
}

variable "profile" {
  type    = string
  default = "mc_profile"
}

variable "state_bucket_name" {
  type        = string
  description = "Globally-unique S3 bucket name for Terraform state."
  default     = "mc-sales-tfstate"
}

variable "lock_table_name" {
  type    = string
  default = "mc-sales-tflock"
}

resource "aws_s3_bucket" "state" {
  bucket = var.state_bucket_name

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "aws:kms"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket                  = aws_s3_bucket.state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_dynamodb_table" "lock" {
  name         = var.lock_table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }
}

output "backend_hcl" {
  description = "Paste these into ../backend.hcl"
  value       = <<-EOT
    bucket         = "${aws_s3_bucket.state.id}"
    key            = "mc-sales/terraform.tfstate"
    region         = "${var.region}"
    dynamodb_table = "${aws_dynamodb_table.lock.name}"
    encrypt        = true
    profile        = "${var.profile}"
  EOT
}
