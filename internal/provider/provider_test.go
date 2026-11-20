package provider

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){
	"shieldome": providerserver.NewProtocol6WithError(New("test")()),
}

func init() { pollInterval = 20 * time.Millisecond }

func providerBlock(url, key string) string {
	return fmt.Sprintf("provider \"shieldome\" {\n  base_url = %q\n  api_key = %q\n}\n", url, key)
}

const baseConfig = `
resource "shieldome_site" "shop" {
  domain = "www.shop.example.com"
}

resource "shieldome_scheduled_scan" "weekly" {
  target_url     = "https://www.shop.example.com"
  interval_hours = 168
  webhook_url    = "https://hooks.example.com/x"
%s
}

resource "shieldome_scan_profile" "staging" {
  name           = "staging"
  target_url     = "https://staging.shop.example.com"
  auth_header    = "Bearer abc"
  custom_headers = %s
}

data "shieldome_latest_scan" "shop" {
  target_url = "https://www.shop.example.com"
}
`

func TestFullLifecycle(t *testing.T) {
	f, srv := newFakeAPI(t)
	f.scans = append(f.scans, map[string]any{"id": "scan-1", "target_url": "https://www.shop.example.com",
		"status": "completed", "scan_type": "both", "completed_at": "2026-11-19T08:00:00",
		"summary": map[string]any{"risk_score": 42.0,
			"vulnerability_counts": map[string]any{"CRITICAL": 0, "HIGH": 2, "MEDIUM": 3, "LOW": 1}}})
	pb := providerBlock(srv.URL, f.key)
	verification := `
resource "shieldome_site_verification" "shop" {
  site_id = shieldome_site.shop.id
  timeout = "1s"
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: pb + fmt.Sprintf(baseConfig, "", `{ "X-Env" = "staging" }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("shieldome_site.shop", "verified", "false"),
					resource.TestCheckResourceAttr("shieldome_site.shop", "verify_record_name", "_shieldome-verify.shop.example.com"),
					resource.TestMatchResourceAttr("shieldome_site.shop", "verify_record_value", regexp.MustCompile(`^shieldome-verify=tok`)),
					resource.TestCheckResourceAttr("shieldome_scheduled_scan.weekly", "scan_type", "both"),
					resource.TestCheckResourceAttr("shieldome_scheduled_scan.weekly", "enabled", "true"),
					resource.TestCheckResourceAttr("shieldome_scheduled_scan.weekly", "notify_email", ""),
					resource.TestCheckResourceAttr("shieldome_scan_profile.staging", "custom_headers.X-Env", "staging"),
					resource.TestCheckResourceAttr("data.shieldome_latest_scan.shop", "found", "true"),
					resource.TestCheckResourceAttr("data.shieldome_latest_scan.shop", "high", "2"),
					resource.TestCheckResourceAttr("data.shieldome_latest_scan.shop", "risk_score", "42"),
				),
			},
			{
				// the TXT record is published (by a DNS provider in real use)
				PreConfig: func() {
					f.mu.Lock()
					defer f.mu.Unlock()
					for _, s := range f.sites {
						rec := f.record(s)
						f.published[rec["name"]] = rec["value"]
					}
				},
				Config: pb + fmt.Sprintf(baseConfig, "", `{ "X-Env" = "staging" }`) + verification,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("shieldome_site_verification.shop", "verified_at", "2026-11-20T10:00:00"),
				),
			},
			{
				// in-place updates: scan type, disable, headers, interval unchanged
				Config: pb + fmt.Sprintf(baseConfig, `  scan_type = "vuln"
  enabled   = false`, `{ "X-Env" = "prod", "X-Waf" = "allow" }`) + verification,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("shieldome_scheduled_scan.weekly", "scan_type", "vuln"),
					resource.TestCheckResourceAttr("shieldome_scheduled_scan.weekly", "enabled", "false"),
					// interval not sent: next_run kept
					resource.TestCheckResourceAttr("shieldome_scheduled_scan.weekly", "next_run", "2026-11-21T10:00:00"),
					resource.TestCheckResourceAttr("shieldome_scan_profile.staging", "custom_headers.%", "2"),
					resource.TestCheckResourceAttr("shieldome_site.shop", "verified", "true"),
				),
			},
			{
				ResourceName:      "shieldome_scheduled_scan.weekly",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:            "shieldome_scan_profile.staging",
				ImportState:             true,
				ImportStateVerify:       true,
			},
		},
		CheckDestroy: func(_ *terraform.State) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.scheds) != 0 || len(f.profiles) != 0 {
				return fmt.Errorf("left behind: %d schedules, %d profiles", len(f.scheds), len(f.profiles))
			}
			if len(f.sites) != 1 {
				return fmt.Errorf("the site must stay in the account (removal is by ticket), got %d", len(f.sites))
			}
			return nil
		},
	})
}

func TestVerificationTimesOutWithTheExpectedRecord(t *testing.T) {
	f, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(srv.URL, f.key) + `
resource "shieldome_site" "s" { domain = "a.example.com" }
resource "shieldome_site_verification" "s" {
  site_id = shieldome_site.s.id
  timeout = "100ms"
}
`,
			ExpectError: regexp.MustCompile(`(?s)not verified in time.*_shieldome-verify\.a\.example\.com`),
		}},
	})
}

func TestExistingSiteAsksForImport(t *testing.T) {
	f, srv := newFakeAPI(t)
	f.sites["site-99"] = map[string]any{"id": "site-99", "domain": "taken.example.com", "verification_active": false}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(srv.URL, f.key) + `resource "shieldome_site" "s" { domain = "taken.example.com" }`,
			ExpectError: regexp.MustCompile(`(?s)already an authorized site.*terraform import`),
		}},
	})
}

func TestDomainMustBeBare(t *testing.T) {
	f, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(srv.URL, f.key) + `resource "shieldome_site" "s" { domain = "https://Shop.example.com/x" }`,
			ExpectError: regexp.MustCompile(`Use "shop.example.com"`),
		}},
	})
}

func TestWrongKeyIsAClearError(t *testing.T) {
	_, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config:      providerBlock(srv.URL, "shd_wrong") + `data "shieldome_latest_scan" "x" { target_url = "https://a.example.com" }`,
			ExpectError: regexp.MustCompile(`check api_key and\s+base_url`),
		}},
	})
}

func TestNoScanYet(t *testing.T) {
	f, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{{
			Config: providerBlock(srv.URL, f.key) + `data "shieldome_latest_scan" "x" { target_url = "https://new.example.com" }`,
			Check:  resource.TestCheckResourceAttr("data.shieldome_latest_scan.x", "found", "false"),
		}},
	})
}
