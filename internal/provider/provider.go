// Package provider implements the Shieldome Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/shieldome/terraform-provider-shieldome/internal/client"
)

const defaultBaseURL = "https://app.shieldome.com"

var _ provider.Provider = &shieldomeProvider{}

type shieldomeProvider struct {
	version string
}

type providerModel struct {
	APIKey  types.String `tfsdk:"api_key"`
	BaseURL types.String `tfsdk:"base_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &shieldomeProvider{version: version} }
}

func (p *shieldomeProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "shieldome"
	resp.Version = p.version
}

func (p *shieldomeProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Shieldome authorized sites, domain verification, scheduled scans and scan profiles, and read scan results.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Shieldome API key (Account -> API Keys). Can also be set with SHIELDOME_API_KEY.",
				Optional:    true,
				Sensitive:   true,
			},
			"base_url": schema.StringAttribute{
				Description: "API base URL. Defaults to " + defaultBaseURL + " or SHIELDOME_BASE_URL (self-hosted installations).",
				Optional:    true,
			},
		},
	}
}

func (p *shieldomeProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.APIKey.IsUnknown() || cfg.BaseURL.IsUnknown() {
		resp.Diagnostics.AddError("Unknown provider configuration",
			"api_key and base_url must be known when the provider is configured.")
		return
	}
	key := os.Getenv("SHIELDOME_API_KEY")
	if !cfg.APIKey.IsNull() {
		key = cfg.APIKey.ValueString()
	}
	base := os.Getenv("SHIELDOME_BASE_URL")
	if !cfg.BaseURL.IsNull() {
		base = cfg.BaseURL.ValueString()
	}
	if base == "" {
		base = defaultBaseURL
	}
	if key == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Missing API key",
			"Set api_key in the provider block or the SHIELDOME_API_KEY environment variable.")
		return
	}
	c := client.New(base, key, p.version)
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *shieldomeProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewSiteResource,
		NewSiteVerificationResource,
		NewScheduledScanResource,
		NewScanProfileResource,
	}
}

func (p *shieldomeProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewLatestScanDataSource,
	}
}

// clientFrom is the shared Configure body of every resource and data source.
func clientFrom(data any, diags interface{ AddError(string, string) }) *client.Client {
	if data == nil {
		return nil // provider not configured yet (validation phase)
	}
	c, ok := data.(*client.Client)
	if !ok {
		diags.AddError("Unexpected provider data", "internal error: provider data is not a Shieldome client")
		return nil
	}
	return c
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
