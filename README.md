# Terraform provider for Shieldome

Manage [Shieldome](https://shieldome.com) from Terraform or OpenTofu: authorized sites and their
DNS ownership verification, scheduled scans and scan profiles, and read scan results to gate a
deployment.

```hcl
terraform {
  required_providers {
    shieldome = { source = "shieldome/shieldome" }
  }
}

provider "shieldome" {}   # SHIELDOME_API_KEY from the environment

resource "shieldome_site" "shop" {
  domain = "shop.example.com"
}

# Ownership proof published with your DNS provider in the same apply
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

resource "shieldome_scheduled_scan" "weekly" {
  target_url     = "https://shop.example.com"
  interval_hours = 168
  webhook_url    = var.slack_webhook
}
```

See [docs/](docs/) for every resource and data source, and [examples/](examples/).

## Resources and data sources

| Name | What it manages |
| --- | --- |
| `shieldome_site` | A domain the account may scan, and the DNS TXT record that proves ownership |
| `shieldome_site_verification` | Waits until Shieldome confirms the ownership proof |
| `shieldome_scheduled_scan` | A recurring scan (Gold-tier plan) |
| `shieldome_scan_profile` | Saved scan settings, including authentication and custom headers |
| `shieldome_latest_scan` (data) | The newest completed scan of a URL: risk score and finding counts |

Authorized sites cannot be removed self-service, so destroying a `shieldome_site` removes it
from the Terraform state only and warns; removal from the account is by support ticket.

## Development

Requires Go 1.23 and Terraform or OpenTofu.

```sh
go build ./...
# provider tests against an in-memory Shieldome API, through the real CLI
TF_ACC_TERRAFORM_PATH=$(which tofu) TF_ACC_PROVIDER_HOST=registry.opentofu.org \
  TF_ACC_PROVIDER_NAMESPACE=hashicorp go test ./...
```

To try a local build against a Shieldome instance, point a CLI config at the binary:

```hcl
# ~/.terraformrc (Terraform) or a file named by TF_CLI_CONFIG_FILE
provider_installation {
  dev_overrides { "registry.terraform.io/shieldome/shieldome" = "/path/to/build/dir" }
  direct {}
}
```

and set `base_url` (or `SHIELDOME_BASE_URL`) for a self-hosted instance.

## Releasing

Tagging `vX.Y.Z` runs `.github/workflows/release.yml`: goreleaser builds every platform, signs
the checksums with the GPG key in the `GPG_PRIVATE_KEY` / `PASSPHRASE` secrets, and publishes a
GitHub release that the Terraform and OpenTofu registries pick up.
