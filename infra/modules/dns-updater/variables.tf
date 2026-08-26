variable "env" { type = string }
variable "domain_name" { type = string }
variable "route53_zone_id" { type = string }

variable "subdomain" {
  type        = string
  default     = "api"
  description = "Origin hostname is <subdomain>.<domain_name>."
}

variable "cluster_arn" { type = string }

variable "ttl" {
  type    = number
  default = 15
}
