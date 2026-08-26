output "cloudfront_domain" { value = aws_cloudfront_distribution.this.domain_name }
output "distribution_id" { value = aws_cloudfront_distribution.this.id }
output "spa_bucket_name" { value = aws_s3_bucket.spa.id }
