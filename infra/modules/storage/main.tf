# App data bucket: uploads, exports, weekly tree-table dumps (§9). IA lifecycle
# at 90 days (§11). Private; the task reaches it via its IAM role + the S3
# gateway endpoint (no internet). Separate from the SPA bucket so the compute
# module can depend on it without creating a cycle with the CDN.

locals {
  name = "mc-sales-app-${var.env}"
}

resource "aws_s3_bucket" "app" {
  bucket = local.name
}

resource "aws_s3_bucket_public_access_block" "app" {
  bucket                  = aws_s3_bucket.app.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "app" {
  bucket = aws_s3_bucket.app.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "app" {
  bucket = aws_s3_bucket.app.id
  rule {
    id     = "ia-90d"
    status = "Enabled"
    filter {}
    transition {
      days          = 90
      storage_class = "STANDARD_IA"
    }
  }
}
