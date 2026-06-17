locals {
  dns_zone_name_normalized  = trimsuffix(var.dns_zone_name, ".")
  app_domain_normalized     = trimsuffix(var.app_domain, ".")
  hoppify_domain_normalized = trimsuffix(coalesce(var.hoppify_domain, "hoppify.${local.dns_zone_name_normalized}"), ".")
  grafana_domain_normalized = trimsuffix(coalesce(var.grafana_domain, "grafana.${local.app_domain_normalized}"), ".")

  app_dns_record_name = (
    local.app_domain_normalized == local.dns_zone_name_normalized
    ? "@"
    : trimsuffix(local.app_domain_normalized, ".${local.dns_zone_name_normalized}")
  )

  grafana_dns_record_name = (
    local.grafana_domain_normalized == local.dns_zone_name_normalized
    ? "@"
    : trimsuffix(local.grafana_domain_normalized, ".${local.dns_zone_name_normalized}")
  )

  hoppify_dns_record_name = (
    local.hoppify_domain_normalized == local.dns_zone_name_normalized
    ? "@"
    : trimsuffix(local.hoppify_domain_normalized, ".${local.dns_zone_name_normalized}")
  )
}

check "app_domain_in_dns_zone" {
  assert {
    condition = (
      local.app_domain_normalized == local.dns_zone_name_normalized ||
      endswith(local.app_domain_normalized, ".${local.dns_zone_name_normalized}")
    )
    error_message = "app_domain must be equal to dns_zone_name or be a subdomain of dns_zone_name."
  }
}

check "grafana_domain_in_dns_zone" {
  assert {
    condition = (
      local.grafana_domain_normalized == local.dns_zone_name_normalized ||
      endswith(local.grafana_domain_normalized, ".${local.dns_zone_name_normalized}")
    )
    error_message = "grafana_domain must be equal to dns_zone_name or be a subdomain of dns_zone_name."
  }
}

check "hoppify_domain_in_dns_zone" {
  assert {
    condition = (
      local.hoppify_domain_normalized == local.dns_zone_name_normalized ||
      endswith(local.hoppify_domain_normalized, ".${local.dns_zone_name_normalized}")
    )
    error_message = "hoppify_domain must be equal to dns_zone_name or be a subdomain of dns_zone_name."
  }
}

resource "hcloud_zone" "primary" {
  name              = local.dns_zone_name_normalized
  mode              = "primary"
  ttl               = var.dns_ttl
  delete_protection = var.dns_delete_protection
  labels            = local.labels
}

resource "hcloud_zone_rrset" "app_a" {
  zone = hcloud_zone.primary.name
  name = local.app_dns_record_name
  type = "A"
  ttl  = var.dns_ttl

  records = [
    {
      value   = hcloud_primary_ip.ipv4.ip_address
      comment = "fsqr API edge IPv4"
    }
  ]

  labels = local.labels
}

resource "hcloud_zone_rrset" "grafana_a" {
  zone = hcloud_zone.primary.name
  name = local.grafana_dns_record_name
  type = "A"
  ttl  = var.dns_ttl

  records = [
    {
      value   = hcloud_primary_ip.ipv4.ip_address
      comment = "fsqr Grafana edge IPv4"
    }
  ]

  labels = local.labels
}

resource "hcloud_zone_rrset" "hoppify_a" {
  zone = hcloud_zone.primary.name
  name = local.hoppify_dns_record_name
  type = "A"
  ttl  = var.dns_ttl

  records = [
    {
      value   = hcloud_primary_ip.ipv4.ip_address
      comment = "Hoppify edge IPv4"
    }
  ]

  labels = local.labels
}
