package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/shieldome/terraform-provider-shieldome/internal/client"
)

var _ resource.Resource = &siteVerificationResource{}

// pollInterval is a variable so tests can shorten it.
var pollInterval = 15 * time.Second

type siteVerificationResource struct{ c *client.Client }

type siteVerificationModel struct {
	ID         types.String `tfsdk:"id"`
	SiteID     types.String `tfsdk:"site_id"`
	Timeout    types.String `tfsdk:"timeout"`
	VerifiedAt types.String `tfsdk:"verified_at"`
}

func NewSiteVerificationResource() resource.Resource { return &siteVerificationResource{} }

func (r *siteVerificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_verification"
}

func (r *siteVerificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Waits until Shieldome confirms ownership of a shieldome_site through its DNS TXT " +
			"record. Create the record first (depends_on), e.g. with the cloudflare or aws provider. " +
			"If the verification later stops being active (record removed, authorization expired), the " +
			"next plan re-creates this resource and checks again.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site_id": schema.StringAttribute{
				Description: "id of the shieldome_site to verify.", Required: true, PlanModifiers: replace,
			},
			"timeout": schema.StringAttribute{
				Description: "How long to keep checking while DNS propagates, as a Go duration. Default 10m.",
				Optional:    true, Computed: true, Default: stringdefault.StaticString("10m"),
			},
			"verified_at": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *siteVerificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *siteVerificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, err := time.ParseDuration(plan.Timeout.ValueString())
	if err != nil || timeout <= 0 {
		resp.Diagnostics.AddError("Invalid timeout", fmt.Sprintf("%q is not a positive duration such as 10m", plan.Timeout.ValueString()))
		return
	}
	siteID := plan.SiteID.ValueString()
	deadline := time.Now().Add(timeout)
	last := ""
	issued := false
	for {
		res, err := r.c.CheckSiteVerification(ctx, siteID)
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && apiErr.Status == 400 && !issued {
			issued = true
			// no token issued yet (site created outside this provider): issue one
			if _, ierr := r.c.InitSiteVerification(ctx, siteID); ierr != nil {
				resp.Diagnostics.AddError("Verification failed", ierr.Error())
				return
			}
			continue
		}
		if err != nil {
			resp.Diagnostics.AddError("Verification failed", err.Error())
			return
		}
		if res.Verified {
			plan.ID = types.StringValue(siteID)
			plan.VerifiedAt = types.StringValue(res.VerifiedAt)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			return
		}
		last = res.Message
		if time.Now().Add(pollInterval).After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Verification cancelled", ctx.Err().Error())
			return
		case <-time.After(pollInterval):
		}
	}
	site, _ := r.c.GetSite(ctx, siteID)
	hint := ""
	if site != nil && site.VerifyRecord != nil {
		hint = fmt.Sprintf(" Expected TXT record %s = %q.", site.VerifyRecord.Name, site.VerifyRecord.Value)
	}
	resp.Diagnostics.AddError("Domain not verified in time",
		fmt.Sprintf("Shieldome did not find the ownership proof within %s.%s %s", timeout, hint, last))
}

func (r *siteVerificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st siteVerificationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	site, err := r.c.GetSite(ctx, st.SiteID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading site failed", err.Error())
		return
	}
	if !site.VerificationActive {
		resp.State.RemoveResource(ctx) // planned again: re-check on the next apply
		return
	}
	if site.VerifiedAt != nil {
		st.VerifiedAt = types.StringValue(*site.VerifiedAt)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *siteVerificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// only timeout can change in place; it matters for Create alone
	var plan siteVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *siteVerificationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Verification is a fact about the domain, not an object to delete: the
	// site stays verified while its proof is in place.
}
