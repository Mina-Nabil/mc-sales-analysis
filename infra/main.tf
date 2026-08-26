# Root — wires the modules. Read top-to-bottom, it is the whole architecture.

module "network" {
  source    = "./modules/network"
  env       = var.env
  region    = var.region
  use_proxy = var.use_proxy
}

module "storage" {
  source = "./modules/storage"
  env    = var.env
}

module "data" {
  source = "./modules/data"
  env    = var.env

  subnet_ids            = module.network.private_subnet_ids
  db_security_group_id  = module.network.db_sg_id
  instance_class        = var.db_instance_class
  allocated_storage     = var.db_allocated_storage
  max_allocated_storage = var.db_max_allocated_storage
  admin_password        = var.admin_password
}

module "compute" {
  source = "./modules/compute"
  env    = var.env
  region = var.region

  subnet_ids          = module.network.public_subnet_ids
  task_security_group = module.network.task_sg_id
  cloud_map_namespace = module.network.cloud_map_namespace_id

  image_tag   = var.image_tag
  task_cpu    = var.task_cpu
  task_memory = var.task_memory

  database_url_ssm_arn = module.data.database_url_ssm_arn
  admin_email          = var.admin_email
  admin_password_ssm   = module.data.admin_password_ssm_arn
  app_bucket_arn       = module.storage.app_bucket_arn
}

# ── Ingress: one of two paths, chosen by var.use_proxy ──────────────────────

# use_proxy=true → t4g.nano Caddy is the stable CloudFront origin (:80).
module "edge_proxy" {
  source = "./modules/edge-proxy"
  count  = var.use_proxy ? 1 : 0
  env    = var.env

  subnet_id         = module.network.public_subnet_ids[0]
  security_group_id = module.network.proxy_sg_id
  upstream_dns      = module.compute.cloud_map_service_name # api.mc.local
  upstream_port     = 8080
}

# use_proxy=false → no box; a Lambda writes the task's public IP into a Route53
# record that CloudFront uses as the /api origin (:8080). ~$7/mo cheaper.
module "dns_updater" {
  source = "./modules/dns-updater"
  count  = var.use_proxy ? 0 : 1
  env    = var.env

  domain_name     = var.domain_name
  route53_zone_id = var.route53_zone_id
  subdomain       = "api"
  cluster_arn     = module.compute.cluster_arn
}

module "edge_cdn" {
  source = "./modules/edge-cdn"
  env    = var.env

  providers = {
    aws           = aws
    aws.us_east_1 = aws.us_east_1
  }

  domain_name     = var.domain_name
  route53_zone_id = var.route53_zone_id

  api_origin_domain    = var.use_proxy ? module.edge_proxy[0].public_dns : module.dns_updater[0].origin_fqdn
  api_origin_http_port = var.use_proxy ? 80 : 8080
}

module "observability" {
  source = "./modules/observability"
  env    = var.env

  # Billing metric lives only in us-east-1.
  providers = { aws = aws.us_east_1 }

  billing_alarm_threshold = var.billing_alarm_threshold
  alarm_email             = var.alarm_email
}
