variable "env" { type = string }
variable "region" { type = string }

variable "subnet_ids" { type = list(string) }
variable "task_security_group" { type = string }
variable "cloud_map_namespace" { type = string }

variable "image_tag" { type = string }
variable "task_cpu" { type = number }
variable "task_memory" { type = number }

variable "database_url_ssm_arn" { type = string }
variable "admin_password_ssm" { type = string }
variable "admin_email" { type = string }

variable "app_bucket_arn" { type = string }
