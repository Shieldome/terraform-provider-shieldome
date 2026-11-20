---
page_title: "shieldome_scan_profile Resource - shieldome"
description: |-
  Saved scan settings, including authentication and custom headers.
---

# shieldome_scan_profile (Resource)

Saved scan settings: target, scan type, excluded paths and authentication. Shieldome stores the
credentials encrypted; they are also in the Terraform state, so protect the state.

## Example Usage

```terraform
resource "shieldome_scan_profile" "staging" {
  name        = "staging"
  target_url  = "https://staging.shop.example.com"
  auth_header = "Bearer ${var.staging_token}"
  custom_headers = {
    "X-Scanner-Allow" = var.waf_allow_token
  }
}
```

## Schema

### Required

- `name` (String)
- `target_url` (String)

### Optional

- `scan_type` (String) `vuln`, `perf`, `both` (default), `api` or `full`.
- `excluded_paths` (String) One per line.
- `auth_cookie` (String, Sensitive) Cookie header value.
- `auth_header` (String, Sensitive) Authorization header value.
- `custom_headers` (Map of String, Sensitive) At most 20 extra request headers.
- `use_playwright` (Boolean) Render in a headless browser. Default false.
- `platform_profile` (String) Platform profile with extra checks, e.g. `wordpress`.

### Read-Only

- `id` (String)

## Import

```shell
terraform import shieldome_scan_profile.staging <profile id>
```
