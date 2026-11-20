package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/shieldome/terraform-provider-shieldome/internal/client"
)

var (
	_ resource.Resource                = &siteResource{}
	_ resource.ResourceWithImportState = &siteResource{}
)

type siteResource struct{ c *client.Client }

type siteModel struct {
	ID                types.String `tfsdk:"id"`
	Domain            types.String `tfsdk:"domain"`
	Verified          types.Bool   `tfsdk:"verified"`
	VerifyMethod      types.String `tfsdk:"verify_method"`
	VerifyRecordName  types.String `tfsdk:"verify_record_name"`
	VerifyRecordValue types.String `tfsdk:"verify_record_value"`
}

func NewSiteResource() resource.Resource { return &siteResource{} }

func (r *siteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (r *siteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	resp.Schema = schema.Schema{
		Description: "A domain the account is authorized to scan. Creating it also issues the DNS TXT " +
			"record that proves ownership (verify_record_name / verify_record_value); publish that record " +
			"and add a shieldome_site_verification to enable the active checks. " +
			"Sites cannot be removed self-service: destroying this resource only removes it from the " +
			"Terraform state - open a support ticket to remove the site from the account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"domain": schema.StringAttribute{
				Description:   "Domain, e.g. shop.example.com (no scheme or path). Changing it creates a new site.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"verified": schema.BoolAttribute{
				Description: "Whether ownership is currently verified (active checks enabled).",
				Computed:    true,
			},
			"verify_method": schema.StringAttribute{
				Description: "How the site was verified: dns_txt, file, wp_plugin or contract.",
				Computed:    true,
			},
			"verify_record_name": schema.StringAttribute{
				Description:   "Name of the DNS TXT record to publish, e.g. _shieldome-verify.example.com.",
				Computed:      true,
				PlanModifiers: keep,
			},
			"verify_record_value": schema.StringAttribute{
				Description:   "Value of the DNS TXT record to publish.",
				Computed:      true,
				PlanModifiers: keep,
			},
		},
	}
}

func (r *siteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	return d
}

func (r *siteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domain := normalizeDomain(plan.Domain.ValueString())
	if domain != plan.Domain.ValueString() {
		resp.Diagnostics.AddAttributeError(path.Root("domain"), "Domain must be a bare host name",
			fmt.Sprintf("Use %q (lower case, no scheme, no path).", domain))
		return
	}
	site, err := r.c.CreateSite(ctx, domain)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 409 {
		resp.Diagnostics.AddError("Site already exists",
			fmt.Sprintf("%s is already an authorized site of this account. Import it instead: "+
				"terraform import <address> <site id> (the id is in Settings -> Sites or GET /api/me/allowed-sites).", domain))
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Creating site failed", err.Error())
		return
	}
	// Issue the DNS proof right away so the record can be created in the same apply.
	if _, err := r.c.InitSiteVerification(ctx, site.ID); err != nil {
		resp.Diagnostics.AddWarning("Verification token not issued",
			"The site was created, but no verification token was issued: "+err.Error())
	}
	plan.ID = types.StringValue(site.ID)
	resp.Diagnostics.Append(r.refresh(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *siteResource) refresh(ctx context.Context, m *siteModel) (diags diag.Diagnostics) {
	site, err := r.c.GetSite(ctx, m.ID.ValueString())
	if err != nil {
		diags.AddError("Reading site failed", err.Error())
		return
	}
	m.Domain = types.StringValue(site.Domain)
	m.Verified = types.BoolValue(site.VerificationActive)
	m.VerifyMethod = types.StringValue(strOrEmpty(site.VerifyMethod))
	if site.VerifyRecord != nil {
		m.VerifyRecordName = types.StringValue(site.VerifyRecord.Name)
		m.VerifyRecordValue = types.StringValue(site.VerifyRecord.Value)
	} else {
		m.VerifyRecordName = types.StringValue("")
		m.VerifyRecordValue = types.StringValue("")
	}
	return
}

func (r *siteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.c.GetSite(ctx, st.ID.ValueString()); errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.refresh(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *siteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every configurable attribute forces replacement; nothing to update in place.
	var st siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *siteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	resp.Diagnostics.AddWarning("Site kept in the Shieldome account",
		fmt.Sprintf("Authorized sites cannot be removed self-service. %s was removed from the Terraform "+
			"state only; open a support ticket to remove it from the account.", st.Domain.ValueString()))
}

func (r *siteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
