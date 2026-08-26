# t4g.nano ARM running Caddy — the stable CloudFront origin for /api/*.
# Caddy reverse-proxies to the task via its Cloud Map name (api.mc.local) with a
# short DNS TTL, so an ephemeral task recycle doesn't break the origin. TLS is
# terminated at CloudFront; the edge->proxy hop is plain HTTP, locked to the
# CloudFront prefix list by the security group (see network module).

locals {
  name = "mc-sales-proxy-${var.env}"
}

data "aws_ami" "al2023_arm" {
  most_recent = true
  owners      = ["amazon"]
  filter {
    name   = "name"
    values = ["al2023-ami-*-arm64"]
  }
  filter {
    name   = "architecture"
    values = ["arm64"]
  }
}

resource "aws_instance" "proxy" {
  ami                    = data.aws_ami.al2023_arm.id
  instance_type          = "t4g.nano"
  subnet_id              = var.subnet_id
  vpc_security_group_ids = [var.security_group_id]

  metadata_options {
    http_tokens = "required" # IMDSv2
  }

  root_block_device {
    volume_size = 8
    volume_type = "gp3"
    encrypted   = true
  }

  user_data = <<-EOT
    #!/bin/bash
    set -euxo pipefail
    dnf install -y 'dnf-command(copr)' || true
    dnf install -y caddy || {
      # fallback: static binary
      curl -fsSL -o /usr/bin/caddy "https://caddyserver.com/api/download?os=linux&arch=arm64"
      chmod +x /usr/bin/caddy
      useradd --system --home /var/lib/caddy --shell /usr/sbin/nologin caddy || true
    }
    cat >/etc/caddy/Caddyfile <<'CADDY'
    # HTTP only; TLS is at CloudFront. Trust the X-Forwarded-* from the edge.
    :80 {
      reverse_proxy http://${var.upstream_dns}:${var.upstream_port} {
        lb_policy       round_robin
        health_uri      /api/v1/stats
      }
    }
    CADDY
    systemctl enable --now caddy
  EOT

  tags = { Name = local.name }
}

resource "aws_eip" "proxy" {
  instance = aws_instance.proxy.id
  domain   = "vpc"
  tags     = { Name = local.name }
}
