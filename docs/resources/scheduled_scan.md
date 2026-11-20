---
page_title: "shieldome_scheduled_scan Resource - shieldome"
description: |-
  A recurring Shieldome scan.
---

# shieldome_scheduled_scan (Resource)

A recurring scan. Requires a Gold-tier plan. Active checks run only on verified sites.

## Example Usage

```terraform
resource "shieldome_scheduled_scan" "weekly" {
  target_url     = "https://shop.example.com"
  scan_type      = "both"
  interval_hours = 168
  webhook_url    = var.slack_webhook
  excluded_paths = "/cart\n/checkout"
}
```

## Schema

### Required

- `target_url` (String) URL to scan. Changing it creates a new schedule.

### Optional

- `scan_type` (String) `vuln`, `perf`, `both` (default) or `api`.
- `interval_hours` (Number) Hours between runs, 1 to 8760. Default 24. Changing it moves the next run.
- `enabled` (Boolean) Default true.
- `notify_email` (String) Address to e-mail the results to.
- `webhook_url` (String) Webhook to post the results to.
- `excluded_paths` (String) Paths not to scan, one per line.

### Read-Only

- `id` (String)
- `next_run` (String) Next run (UTC).
- `last_run` (String) Last run (UTC), empty before the first.

## Import

```shell
terraform import shieldome_scheduled_scan.weekly <schedule id>
```
