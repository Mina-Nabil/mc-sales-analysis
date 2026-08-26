output "app_url" {
  value = "https://${var.domain_name}"
}

output "cloudfront_domain" {
  value = module.edge_cdn.cloudfront_domain
}

output "ecr_repository_url" {
  description = "Push the app image here (CI)."
  value       = module.compute.ecr_repository_url
}

output "spa_bucket" {
  description = "aws s3 sync web/dist/ s3://<this>/  for a front-end deploy."
  value       = module.edge_cdn.spa_bucket_name
}

output "cloudfront_distribution_id" {
  description = "Invalidate after a SPA sync."
  value       = module.edge_cdn.distribution_id
}

output "rds_endpoint" {
  value     = module.data.endpoint
  sensitive = true
}

output "proxy_public_ip" {
  value = var.use_proxy ? module.edge_proxy[0].public_ip : null
}

output "api_origin" {
  description = "Hostname CloudFront uses for /api/*."
  value       = var.use_proxy ? module.edge_proxy[0].public_dns : module.dns_updater[0].origin_fqdn
}
