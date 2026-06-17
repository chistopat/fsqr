output "server_id" {
  description = "Hetzner server ID."
  value       = hcloud_server.app.id
}

output "server_name" {
  description = "Hetzner server name."
  value       = hcloud_server.app.name
}

output "ipv4_address" {
  description = "Static primary IPv4 address attached to the server."
  value       = hcloud_primary_ip.ipv4.ip_address
}

output "ssh_command" {
  description = "Command for connecting to the server after cloud-init finishes."
  value       = "ssh -i ${var.ssh_private_key_path} ${var.admin_user}@${hcloud_primary_ip.ipv4.ip_address}"
}

output "app_domain" {
  description = "Public fsqr API hostname managed by Hetzner DNS."
  value       = local.app_domain_normalized
}

output "app_base_url" {
  description = "Public fsqr API base URL."
  value       = "https://${local.app_domain_normalized}"
}

output "grafana_domain" {
  description = "Public Grafana hostname managed by Hetzner DNS."
  value       = local.grafana_domain_normalized
}

output "grafana_base_url" {
  description = "Public Grafana base URL."
  value       = "https://${local.grafana_domain_normalized}"
}

output "hoppify_domain" {
  description = "Public Hoppify hostname managed by Hetzner DNS."
  value       = local.hoppify_domain_normalized
}

output "hoppify_base_url" {
  description = "Public Hoppify base URL."
  value       = "https://${local.hoppify_domain_normalized}"
}

output "dns_zone_nameservers" {
  description = "Authoritative Hetzner nameservers that must be delegated at the domain registrar."
  value       = hcloud_zone.primary.authoritative_nameservers.assigned
}
