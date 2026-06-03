locals {
  name_prefix    = "${var.project_name}-${var.environment}"
  ssh_public_key = trimspace(file(var.ssh_public_key_path))

  labels = {
    project     = var.project_name
    environment = var.environment
    managed_by  = "opentofu"
  }
}

resource "hcloud_ssh_key" "deploy" {
  name       = "${local.name_prefix}-deploy"
  public_key = local.ssh_public_key
  labels     = local.labels
}

resource "hcloud_firewall" "edge" {
  name   = "${local.name_prefix}-edge"
  labels = local.labels

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "22"
    source_ips = var.ssh_source_ips
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "80"
    source_ips = ["0.0.0.0/0"]
  }

  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "443"
    source_ips = ["0.0.0.0/0"]
  }
}

resource "hcloud_primary_ip" "ipv4" {
  name        = "${local.name_prefix}-ipv4"
  location    = var.location
  type        = "ipv4"
  auto_delete = false
  labels      = local.labels
}

resource "hcloud_server" "app" {
  name        = "${local.name_prefix}-app-01"
  image       = var.image
  server_type = var.server_type
  location    = var.location
  ssh_keys    = [hcloud_ssh_key.deploy.id]
  firewall_ids = [
    hcloud_firewall.edge.id,
  ]
  user_data = templatefile("${path.module}/cloud-init.yaml.tftpl", {
    admin_user     = var.admin_user
    ssh_public_key = local.ssh_public_key
  })
  labels = local.labels

  public_net {
    ipv4_enabled = true
    ipv4         = hcloud_primary_ip.ipv4.id
    ipv6_enabled = false
  }
}
