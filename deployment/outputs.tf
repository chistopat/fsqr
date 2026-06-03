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
