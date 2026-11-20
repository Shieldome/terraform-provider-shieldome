---
page_title: "shieldome_site_verification Resource - shieldome"
description: |-
  Waits until Shieldome confirms ownership of a shieldome_site through its DNS TXT record.
---

# shieldome_site_verification (Resource)

Waits until Shieldome confirms ownership of a `shieldome_site` through its DNS TXT record,
checking every 15 seconds until `timeout`. Create the record first with your DNS provider
(`depends_on`). If verification later stops being active (record removed, written authorization
expired), the next plan re-creates this resource and checks again. Destroying it changes nothing
in Shieldome.

## Example Usage

```terraform
resource "aws_route53_record" "shieldome_verify" {
  zone_id = var.zone_id
  name    = shieldome_site.shop.verify_record_name
  type    = "TXT"
  ttl     = 300
  records = [shieldome_site.shop.verify_record_value]
}

resource "shieldome_site_verification" "shop" {
  site_id    = shieldome_site.shop.id
  timeout    = "15m"
  depends_on = [aws_route53_record.shieldome_verify]
}
```

## Schema

### Required

- `site_id` (String) id of the `shieldome_site`.

### Optional

- `timeout` (String) How long to keep checking while DNS propagates, as a duration. Default `10m`.

### Read-Only

- `id` (String) Same as `site_id`.
- `verified_at` (String) When ownership was confirmed (UTC).
