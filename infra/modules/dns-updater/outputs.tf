output "origin_fqdn" {
  description = "Stable hostname CloudFront uses as the /api origin."
  value       = local.record_fqdn
}
