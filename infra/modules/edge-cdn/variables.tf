variable "env" { type = string }
variable "domain_name" { type = string }
variable "route53_zone_id" { type = string }

variable "api_origin_domain" {
  type        = string
  description = "Proxy EIP DNS (use_proxy), or the task's Route53 record (direct)."
}

variable "api_origin_http_port" {
  type        = number
  default     = 80
  description = "80 for the nano proxy; 8080 for the task directly."
}
