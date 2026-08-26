variable "region" {
  type    = string
  default = "eu-central-1"
}

variable "profile" {
  type        = string
  description = "Named AWS CLI profile. Never hard-code credentials."
  default     = "mc_profile"
}

variable "env" {
  type    = string
  default = "prod"
}

# ── DNS / edge ──────────────────────────────────────────────────────────────
variable "domain_name" {
  type        = string
  description = "Public hostname for the app, e.g. sales.motorcity.example."
}

variable "route53_zone_id" {
  type        = string
  description = "ID of the existing Route53 public hosted zone for domain_name."
}

# ── App image ───────────────────────────────────────────────────────────────
variable "image_tag" {
  type        = string
  description = "ECR image tag to deploy (CI sets this; 'latest' for manual)."
  default     = "latest"
}

# ── Sizing (matches §11 exactly) ────────────────────────────────────────────
variable "task_cpu" {
  type    = number
  default = 512 # 0.5 vCPU
}

variable "task_memory" {
  type    = number
  default = 1024 # 1 GB
}

variable "db_instance_class" {
  type    = string
  default = "db.t4g.small"
}

variable "db_allocated_storage" {
  type    = number
  default = 20
}

variable "db_max_allocated_storage" {
  type        = number
  default     = 100
  description = "gp3 storage autoscaling ceiling."
}

# ── Edge ingress toggle (§11 trim lever) ────────────────────────────────────
variable "use_proxy" {
  type        = bool
  default     = true
  description = <<-EOT
    true  → t4g.nano Caddy proxy is the CloudFront /api origin (robust, ~+$7/mo).
    false → CloudFront points /api straight at the task's Cloud Map *public*
            record; drops the nano (~$50/mo) but blips 5xx during task recycles.
  EOT
}

# ── App config / secrets ────────────────────────────────────────────────────
variable "admin_email" {
  type    = string
  default = "admin@motorcity.local"
}

variable "admin_password" {
  type        = string
  sensitive   = true
  description = "Seed-admin password. Set via TF_VAR_admin_password, never in tfvars."
}

variable "billing_alarm_threshold" {
  type        = number
  default     = 115
  description = "USD/month — 2x expected (§11.1)."
}

variable "alarm_email" {
  type        = string
  description = "Where the billing alarm notifies."
}
