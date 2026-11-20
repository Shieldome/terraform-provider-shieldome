package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/shieldome/terraform-provider-shieldome/internal/client"
)

var (
	_ resource.Resource                = &scheduledScanResource{}
	_ resource.ResourceWithImportState = &scheduledScanResource{}
)

type scheduledScanResource struct{ c *client.Client }

type scheduledScanModel struct {
	ID            types.String `tfsdk:"id"`
	TargetURL     types.String `tfsdk:"target_url"`
	ScanType      types.String `tfsdk:"scan_type"`
	IntervalHours types.Int64  `tfsdk:"interval_hours"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	NotifyEmail   types.String `tfsdk:"notify_email"`
	WebhookURL    types.String `tfsdk:"webhook_url"`
	ExcludedPaths types.String `tfsdk:"excluded_paths"`
	NextRun       types.String `tfsdk:"next_run"`
	LastRun       types.String `tfsdk:"last_run"`
}

func NewScheduledScanResource() resource.Resource { return &scheduledScanResource{} }

func (r *scheduledScanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_scan"
}

func (r *scheduledScanResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	optStr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, Optional: true, Computed: true,
			Default: stringdefault.StaticString("")}
	}
	resp.Schema = schema.Schema{
		Description: "A recurring scan (requires a Gold-tier plan). Active checks run only on verified sites.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"target_url": schema.StringAttribute{
				Description:   "URL to scan. Changing it creates a new schedule.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"scan_type": schema.StringAttribute{
				Description: "vuln, perf, both (default) or api.",
				Optional:    true, Computed: true, Default: stringdefault.StaticString("both"),
				Validators: []validator.String{stringvalidator.OneOf("vuln", "perf", "both", "api")},
			},
			"interval_hours": schema.Int64Attribute{
				Description: "Hours between runs, 1 to 8760. Default 24.",
				Optional:    true, Computed: true, Default: int64default.StaticInt64(24),
				Validators: []validator.Int64{int64validator.Between(1, 8760)},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the schedule runs. Default true.",
				Optional:    true, Computed: true, Default: booldefault.StaticBool(true),
			},
			"notify_email":   optStr("Address to e-mail the results to."),
			"webhook_url":    optStr("Webhook (e.g. Slack) to post the results to."),
			"excluded_paths": optStr("Paths not to scan, one per line."),
			"next_run":       schema.StringAttribute{Computed: true, Description: "Next run (UTC)."},
			"last_run":       schema.StringAttribute{Computed: true, Description: "Last run (UTC), empty before the first."},
		},
	}
}

func (r *scheduledScanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (m *scheduledScanModel) fill(s *client.ScheduledScan) {
	m.ID = types.StringValue(s.ID)
	m.TargetURL = types.StringValue(s.TargetURL)
	m.ScanType = types.StringValue(s.ScanType)
	m.IntervalHours = types.Int64Value(s.IntervalHours)
	m.Enabled = types.BoolValue(s.Enabled)
	m.NotifyEmail = types.StringValue(s.NotifyEmail)
	m.WebhookURL = types.StringValue(s.WebhookURL)
	m.ExcludedPaths = types.StringValue(s.ExcludedPaths)
	m.NextRun = types.StringValue(strOrEmpty(s.NextRun))
	m.LastRun = types.StringValue(strOrEmpty(s.LastRun))
}

func (m *scheduledScanModel) patch() map[string]any {
	return map[string]any{
		"scan_type":      m.ScanType.ValueString(),
		"interval_hours": m.IntervalHours.ValueInt64(),
		"enabled":        m.Enabled.ValueBool(),
		"notify_email":   m.NotifyEmail.ValueString(),
		"webhook_url":    m.WebhookURL.ValueString(),
		"excluded_paths": m.ExcludedPaths.ValueString(),
	}
}

func (r *scheduledScanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduledScanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.c.CreateScheduledScan(ctx, client.ScheduledScan{
		TargetURL:     plan.TargetURL.ValueString(),
		ScanType:      plan.ScanType.ValueString(),
		IntervalHours: plan.IntervalHours.ValueInt64(),
		Enabled:       true,
		NotifyEmail:   plan.NotifyEmail.ValueString(),
		WebhookURL:    plan.WebhookURL.ValueString(),
		ExcludedPaths: plan.ExcludedPaths.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Creating scheduled scan failed", err.Error())
		return
	}
	if !plan.Enabled.ValueBool() { // a new schedule always starts enabled
		if s, err = r.c.UpdateScheduledScan(ctx, s.ID, map[string]any{"enabled": false}); err != nil {
			resp.Diagnostics.AddError("Disabling scheduled scan failed", err.Error())
			return
		}
	}
	plan.fill(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scheduledScanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st scheduledScanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.c.GetScheduledScan(ctx, st.ID.ValueString())
	if errors.Is(err, client.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading scheduled scan failed", err.Error())
		return
	}
	st.fill(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *scheduledScanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, st scheduledScanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p := plan.patch()
	// PATCH resets next_run whenever interval_hours is sent: send it only on change
	if plan.IntervalHours.Equal(st.IntervalHours) {
		delete(p, "interval_hours")
	}
	s, err := r.c.UpdateScheduledScan(ctx, st.ID.ValueString(), p)
	if err != nil {
		resp.Diagnostics.AddError("Updating scheduled scan failed", err.Error())
		return
	}
	plan.fill(s)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scheduledScanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st scheduledScanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if err := r.c.DeleteScheduledScan(ctx, st.ID.ValueString()); err != nil && !errors.Is(err, client.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting scheduled scan failed", err.Error())
	}
}

func (r *scheduledScanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

