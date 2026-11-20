---
page_title: "shieldome_site Resource - shieldome"
description: |-
  A domain the account is authorized to scan, and the DNS TXT record that proves ownership.
---

# shieldome_site (Resource)

A domain the account is authorized to scan. Creating it also issues the DNS TXT record that
proves ownership (`verify_record_name` / `verify_record_value`). Publish that record and add a
`shieldome_site_verification` to enable the active checks; without verification Shieldome runs
the passive assessment only.

Authorized sites cannot be removed self-service: destroying this resource only removes it from
the Terraform state, with a warning. Open a support ticket to remove the site from the account.

## Example Usage

```terraform
resource "shieldome_site" "shop" {
  domain = "shop.example.com"
}
```

## Schema

### Required

- `domain` (String) Bare host name, lower case, e.g. `shop.example.com`. Changing it creates a new site.

### Read-Only

- `id` (String) Site id.
- `verified` (Boolean) Whether ownership is currently verified.
- `verify_method` (String) `dns_txt`, `file`, `wp_plugin` or `contract`.
- `verify_record_name` (String) Name of the TXT record to publish.
- `verify_record_value` (String) Value of the TXT record to publish.

## Import

```shell
terraform import shieldome_site.shop <site id>
```

The id is listed by `GET /api/me/allowed-sites`. Creating a site that already exists in the
account fails with a message asking for this import.
