---
page_title: "shieldome_latest_scan Data Source - shieldome"
description: |-
  The newest completed scan of a URL in this account.
---

# shieldome_latest_scan (Data Source)

The newest completed scan of a URL in this account, e.g. to stop a deployment when the site
has critical findings.

## Example Usage

```terraform
data "shieldome_latest_scan" "shop" {
  target_url = "https://shop.example.com"
}

resource "terraform_data" "release" {
  lifecycle {
    precondition {
      condition     = !data.shieldome_latest_scan.shop.found || data.shieldome_latest_scan.shop.critical == 0
      error_message = "shop.example.com has critical findings in its latest Shieldome scan."
    }
  }
}
```

## Schema

### Required

- `target_url` (String) The URL exactly as it was submitted for scanning.

### Read-Only

- `found` (Boolean) false when the URL has no completed scan; the other attributes are then empty or 0.
- `id` (String) Scan id.
- `scan_type` (String)
- `completed_at` (String) UTC.
- `risk_score` (Number) 0 to 100.
- `critical`, `high`, `medium`, `low` (Number) Finding counts by severity.
