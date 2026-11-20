package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/shieldome/terraform-provider-shieldome/internal/client"
)

var (
	_ resource.Resource                = &scanProfileResource{}
	_ resource.ResourceWithImportState = &scanProfileResource{}
)

type scanProfileResource struct{ c *client.Client }

type scanProfileModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	TargetURL       types.String `tfsdk:"target_url"`
	ScanType        types.String `tfsdk:"scan_type"`
	ExcludedPaths   types.String `tfsdk:"excluded_paths"`
	AuthCookie      types.String `tfsdk:"auth_cookie"`
	AuthHeader      types.String `tfsdk:"auth_header"`
	CustomHeaders   types.Map    `tfsdk:"custom_headers"`
	UsePlaywright   types.Bool   `tfsdk:"use_playwright"`
	PlatformProfile types.String `tfsdk:"platform_profile"`
}

func NewScanProfileResource() resource.Resource { return &scanProfileResource{} }

func (r *scanProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scan_profile"
}

func (r *scanProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optStr := func(desc string, sensitive bool) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, Optional: true, Computed: true, Sensitive: sensitive,
			Default: stringdefault.StaticString("")}
	}
	resp.Schema = schema.Schema{
		Description: "Saved scan settings: target, scan type, excluded paths and authentication. Credentials are stored encrypted by Shieldome.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":       schema.StringAttribute{Required: true, Description: "Profile name."},
			"target_url": schema.StringAttribute{Required: true, Description: "URL the profile scans."},
			"scan_type": schema.StringAttribute{
				Description: "vuln, perf, both (default), api or full.",
				Optional:    true, Computed: true, Default: stringdefault.StaticString("both"),
				Validators: []validator.String{stringvalidator.OneOf("vuln", "perf", "both", "api", "full")},
			},
			"excluded_paths": optStr("Paths not to scan, one per line.", false),
			"auth_cookie":    optStr("Cookie header value for authenticated scans.", true),
			"auth_header":    optStr("Authorization header value, e.g. Bearer <token>.", true),
			"custom_headers": schema.MapAttribute{
				Description: "Extra request headers (at most 20), e.g. a WAF allowlist header.",
				ElementType: types.StringType, Optional: true, Computed: true, Sensitive: true,
				Default: mapdefault.StaticValue(types.MapValueMust(types.StringType, nil)),
			},
			"use_playwright": schema.BoolAttribute{
				Description: "Render pages in a headless browser (single-page apps). Default false.",
				Optional:    true, Computed: true, Default: booldefault.StaticBool(false),
			},
			"platform_profile": optStr("Platform profile with extra checks, e.g. wordpress.", false),
		},
	}
}

func (r *scanProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (m *scanProfileModel) body(ctx context.Context) (client.ScanProfile, error) {
	headers := map[string]string{}
	if d := m.CustomHeaders.ElementsAs(ctx, &headers, false); d.HasError() {
		return client.ScanProfile{}, errors.New("custom_headers: values must be strings")
	}
	pp := m.PlatformProfile.ValueString()
	return client.ScanProfile{
		Name: m.Name.ValueString(), TargetURL: m.TargetURL.ValueString(), ScanType: m.ScanType.ValueString(),
		ExcludedPaths: m.ExcludedPaths.ValueString(), AuthCookie: m.AuthCookie.ValueString(),
		AuthHeader: m.AuthHeader.ValueString(), CustomHeaders: headers,
		UsePlaywright: m.UsePlaywright.ValueBool(), PlatformProfile: &pp,
	}, nil
}

func (m *scanProfileModel) fill(p *client.ScanProfile) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.TargetURL = types.StringValue(p.TargetURL)
	m.ScanType = types.StringValue(p.ScanType)
	m.ExcludedPaths = types.StringValue(p.ExcludedPaths)
	m.AuthCookie = types.StringValue(p.AuthCookie)
	m.AuthHeader = types.StringValue(p.AuthHeader)
	h := map[string]string{}
	for k, v := range p.CustomHeaders {
		h[k] = v
	}
	m.CustomHeaders = types.MapValueMust(types.StringType, toAttrMap(h))
	m.UsePlaywright = types.BoolValue(p.UsePlaywright)
	m.PlatformProfile = types.StringValue(strOrEmpty(p.PlatformProfile))
}

func (r *scanProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scanProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := plan.body(ctx)
	if err == nil {
		var p *client.ScanProfile
		if p, err = r.c.CreateScanProfile(ctx, b); err == nil {
			plan.fill(p)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Creating scan profile failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scanProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st scanProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.c.GetScanProfile(ctx, st.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading scan profile failed", err.Error())
		return
	}
	st.fill(p)
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *scanProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, st scanProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	b, err := plan.body(ctx)
	if err == nil {
		var p *client.ScanProfile
		if p, err = r.c.UpdateScanProfile(ctx, st.ID.ValueString(), b); err == nil {
			plan.fill(p)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Updating scan profile failed", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scanProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st scanProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if err := r.c.DeleteScanProfile(ctx, st.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting scan profile failed", err.Error())
	}
}

func (r *scanProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
