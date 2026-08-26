variable "env" { type = string }
variable "subnet_ids" { type = list(string) }
variable "db_security_group_id" { type = string }
variable "instance_class" { type = string }
variable "allocated_storage" { type = number }
variable "max_allocated_storage" { type = number }

variable "admin_password" {
  type      = string
  sensitive = true
}
