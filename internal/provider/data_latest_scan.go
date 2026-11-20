package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/shieldome/terraform-provider-shieldome/internal/client"
)

var _ datasource.DataSource = &latestScanDataSource{}

type latestScanDataSource struct{ c *client.Client }

type latestScanModel struct {
	TargetURL   types.String  `tfsdk:"target_url"`
	Found       types.Bool    `tfsdk:"found"`
	ID          types.String  `tfsdk:"id"`
	ScanType    types.String  `tfsdk:"scan_type"`
	CompletedAt types.String  `tfsdk:"completed_at"`
	RiskScore   types.Float64 `tfsdk:"risk_score"`
	Critical    types.Int64   `tfsdk:"critical"`
	High        types.Int64   `tfsdk:"high"`
	Medium      types.Int64   `tfsdk:"medium"`
	Low         types.Int64   `tfsdk:"low"`
}

func NewLatestScanDataSource() datasource.DataSource { return &latestScanDataSource{} }

func (d *latestScanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_latest_scan"
}

func (d *latestScanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	n := func(desc string) schema.Int64Attribute { return schema.Int64Attribute{Computed: true, Description: desc} }
	resp.Schema = schema.Schema{
		Description: "The newest completed scan of a URL in this account, e.g. to fail a deployment with a " +
			"check or precondition when it has critical findings.",
		Attributes: map[string]schema.Attribute{
			"target_url": schema.StringAttribute{Required: true,
				Description: "The scanned URL exactly as it was submitted, e.g. https://shop.example.com."},
			"found":        schema.BoolAttribute{Computed: true, Description: "false when the URL has no completed scan."},
			"id":           schema.StringAttribute{Computed: true, Description: "Scan id."},
			"scan_type":    schema.StringAttribute{Computed: true},
			"completed_at": schema.StringAttribute{Computed: true, Description: "UTC timestamp."},
			"risk_score":   schema.Float64Attribute{Computed: true, Description: "0 (no risk) to 100."},
			"critical":     n("Critical findings."),
			"high":         n("High findings."),
			"medium":       n("Medium findings."),
			"low":          n("Low findings."),
		},
	}
}

func (d *latestScanDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// count reads a severity count whatever its key case (CRITICAL / critical).
func count(m map[string]any, sev string) int64 {
	for k, v := range m {
		if strings.EqualFold(k, sev) {
			if f, ok := v.(float64); ok {
				return int64(f)
			}
		}
	}
	return 0
}

func (d *latestScanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m latestScanModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := d.c.LatestCompletedScan(ctx, m.TargetURL.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Reading scans failed", err.Error())
		return
	}
	m.Found = types.BoolValue(s != nil)
	if s == nil {
		s = &client.Scan{}
	}
	m.ID = types.StringValue(s.ID)
	m.ScanType = types.StringValue(s.ScanType)
	m.CompletedAt = types.StringValue(strOrEmpty(s.CompletedAt))
	m.RiskScore = types.Float64Value(s.Summary.RiskScore)
	c := s.Summary.VulnerabilityCounts
	m.Critical = types.Int64Value(count(c, "critical"))
	m.High = types.Int64Value(count(c, "high"))
	m.Medium = types.Int64Value(count(c, "medium"))
	m.Low = types.Int64Value(count(c, "low"))
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

func toAttrMap(m map[string]string) map[string]attr.Value {
	out := make(map[string]attr.Value, len(m))
	for k, v := range m {
		out[k] = types.StringValue(v)
	}
	return out
}
