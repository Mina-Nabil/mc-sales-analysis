output "ecr_repository_url" { value = aws_ecr_repository.app.repository_url }

output "cluster_arn" { value = aws_ecs_cluster.this.arn }
output "cluster_name" { value = aws_ecs_cluster.this.name }
output "service_name" { value = aws_ecs_service.app.name }

output "cloud_map_service_name" {
  description = "Private DNS the proxy resolves to reach the task (use_proxy=true)."
  value       = "api.mc.local"
}
