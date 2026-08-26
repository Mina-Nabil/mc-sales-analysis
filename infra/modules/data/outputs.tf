output "endpoint" { value = aws_db_instance.this.address }
output "database_url_ssm_arn" { value = aws_ssm_parameter.database_url.arn }
output "admin_password_ssm_arn" { value = aws_ssm_parameter.admin_password.arn }
