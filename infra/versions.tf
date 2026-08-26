terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source                = "hashicorp/aws"
      version               = "~> 5.60"
      configuration_aliases = [aws.us_east_1]
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.4"
    }
  }

  # Configured out-of-band via `terraform init -backend-config=backend.hcl`
  # (written by the bootstrap module). Kept partial so the same code applies
  # to any account.
  backend "s3" {}
}
