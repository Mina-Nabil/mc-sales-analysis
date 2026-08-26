variable "env" { type = string }
variable "region" { type = string }

variable "use_proxy" {
  type        = bool
  description = "Whether the nano proxy path is in use (gates the proxy SG + rules)."
}
