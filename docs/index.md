---
page_title: "Shieldome Provider"
description: |-
  Manage Shieldome authorized sites, domain verification, scheduled scans and scan profiles, and read scan results.
---

# Shieldome Provider

Manage [Shieldome](https://shieldome.com) web vulnerability scanning as code.

## Example Usage

```terraform
provider "shieldome" {
  # api_key from SHIELDOME_API_KEY
}
```

## Authentication

Create an API key in the Shieldome app (Account -> API Keys) and pass it as `api_key` or in the
`SHIELDOME_API_KEY` environment variable. The key acts as your account: keep it in a secret store.

## Schema

### Optional

- `api_key` (String, Sensitive) Shieldome API key. Defaults to `SHIELDOME_API_KEY`.
- `base_url` (String) API base URL. Defaults to `SHIELDOME_BASE_URL`, then `https://app.shieldome.com`
  (set it for a self-hosted installation).
