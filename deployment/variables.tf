variable "hcloud_token" {
  description = "Hetzner Cloud API token. Prefer TF_VAR_hcloud_token from .env; do not store it in tfvars."
  type        = string
  sensitive   = true
}

variable "project_name" {
  description = "Project label/name prefix."
  type        = string
  default     = "fsqr"
}

variable "environment" {
  description = "Deployment environment label/name prefix."
  type        = string
  default     = "prod"
}

variable "location" {
  description = "Hetzner location. hel1 is Helsinki, Finland."
  type        = string
  default     = "hel1"
}

variable "server_type" {
  description = "Hetzner server type."
  type        = string
  default     = "cpx42"
}

variable "image" {
  description = "Hetzner image name. docker-ce is the app image with Docker preinstalled."
  type        = string
  default     = "docker-ce"
}

variable "admin_user" {
  description = "Non-root SSH user created by cloud-init."
  type        = string
  default     = "deploy"
}

variable "ssh_public_key_path" {
  description = "Path to the deploy SSH public key, relative to deployment/ by default."
  type        = string
  default     = "../.ssh/fsqr_hcloud_ed25519.pub"
}

variable "ssh_private_key_path" {
  description = "Path to the deploy SSH private key, used only for SSH command output."
  type        = string
  default     = "../.ssh/fsqr_hcloud_ed25519"
}

variable "ssh_source_ips" {
  description = "IPv4 CIDRs allowed to reach SSH. Default is open until a stable operator IP is known."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "dns_zone_name" {
  description = "Primary Hetzner DNS zone that contains the public fsqr hostname."
  type        = string
}

variable "app_domain" {
  description = "Public hostname for the fsqr API. Must be inside dns_zone_name."
  type        = string
}

variable "grafana_domain" {
  description = "Public hostname for Grafana. Defaults to grafana.<app_domain> and must be inside dns_zone_name."
  type        = string
  default     = null
  nullable    = true
}

variable "dns_ttl" {
  description = "TTL in seconds for fsqr DNS records."
  type        = number
  default     = 300
}

variable "dns_delete_protection" {
  description = "Protect the Hetzner DNS zone from accidental deletion."
  type        = bool
  default     = true
}
