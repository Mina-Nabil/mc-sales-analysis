output "vpc_id" { value = aws_vpc.this.id }
output "public_subnet_ids" { value = aws_subnet.public[*].id }
output "private_subnet_ids" { value = aws_subnet.private[*].id }

output "proxy_sg_id" { value = one(aws_security_group.proxy[*].id) }
output "task_sg_id" { value = aws_security_group.task.id }
output "db_sg_id" { value = aws_security_group.db.id }

output "cloud_map_namespace_id" { value = aws_service_discovery_private_dns_namespace.this.id }
