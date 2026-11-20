terraform {
  required_providers {
    shieldome  = { source = "shieldome/shieldome" }
    cloudflare = { source = "cloudflare/cloudflare", version = "~> 4.0" }
  }
}

provider "shieldome" {} # SHIELDOME_API_KEY from the environment

variable "zone_id" { type = string }
variable "slack_webhook" {
  type      = string
  sensitive = true
}

# 1. The domain, and the DNS record that proves you own it
resource "shieldome_site" "shop" {
  domain = "shop.example.com"
}

resource "cloudflare_record" "shieldome_verify" {
  zone_id = var.zone_id
  name    = shieldome_site.shop.verify_record_name
  type    = "TXT"
  content = shieldome_site.shop.verify_record_value
}

resource "shieldome_site_verification" "shop" {
  site_id    = shieldome_site.shop.id
  depends_on = [cloudflare_record.shieldome_verify]
}

# 2. A weekly scan that posts to Slack
resource "shieldome_scheduled_scan" "weekly" {
  target_url     = "https://shop.example.com"
  interval_hours = 168
  webhook_url    = var.slack_webhook
  excluded_paths = "/cart\n/checkout"
  depends_on     = [shieldome_site_verification.shop]
}

# 3. Stop a release while the site has critical findings
data "shieldome_latest_scan" "shop" {
  target_url = "https://shop.example.com"
}

resource "terraform_data" "release_gate" {
  lifecycle {
    precondition {
      condition     = !data.shieldome_latest_scan.shop.found || data.shieldome_latest_scan.shop.critical == 0
      error_message = "shop.example.com has critical findings in its latest Shieldome scan."
    }
  }
}
