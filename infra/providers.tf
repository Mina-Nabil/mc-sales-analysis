# Primary region — everything except the CloudFront ACM cert.
provider "aws" {
  region  = var.region
  profile = var.profile

  default_tags {
    tags = {
      Project   = "mc-sales-analytics"
      ManagedBy = "terraform"
      Env       = var.env
    }
  }
}

# CloudFront requires its ACM certificate in us-east-1, regardless of where the
# rest of the stack lives. This alias exists only for that cert (edge-cdn module).
provider "aws" {
  alias   = "us_east_1"
  region  = "us-east-1"
  profile = var.profile

  default_tags {
    tags = {
      Project   = "mc-sales-analytics"
      ManagedBy = "terraform"
      Env       = var.env
    }
  }
}
