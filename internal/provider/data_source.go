package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/bogware/terraform-provider-mergewf/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &apiDataSource{}
var _ datasource.DataSourceWithConfigure = &apiDataSource{}
var _ datasource.DataSourceWithValidateConfig = &apiDataSource{}

type apiDataSource struct {
	spec   dataSpec
	client *client.Client
}

func (d *apiDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.Name
}
func (d *apiDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := map[string]schema.Attribute{"id": schema.StringAttribute{Computed: true, MarkdownDescription: "Stable hash of the request path and filters."}, "result_json": schema.StringAttribute{Computed: true, MarkdownDescription: "API response encoded as JSON. For list endpoints this is a JSON array containing all matching results across every page. Use jsondecode to consume it. SCIM discovery endpoints retain their response envelopes."}}
	for _, p := range d.spec.Parameters {
		node := contract.Resolve(p.Schema)
		desc := "API " + p.In + " parameter `" + p.Name + "`."
		if p.Name == "limit" || p.Name == "count" {
			desc += " Page size, not a cap on total returned results."
		}
		switch node.Kind() {
		case "integer":
			attrs[snake(p.Name)] = schema.Int64Attribute{Required: p.Required, Optional: !p.Required, MarkdownDescription: desc}
		case "boolean":
			attrs[snake(p.Name)] = schema.BoolAttribute{Required: p.Required, Optional: !p.Required, MarkdownDescription: desc}
		default:
			attrs[snake(p.Name)] = schema.StringAttribute{Required: p.Required, Optional: !p.Required, MarkdownDescription: desc}
		}
	}
	resp.Schema = schema.Schema{MarkdownDescription: "Reads `GET " + d.spec.Route + "` from Merge Workforce. Pagination is automatic for list endpoints. API filters use snake_case attribute names.", Attributes: attrs}
}
func (d *apiDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Provider configuration error", "Unexpected client type.")
		return
	}
	d.client = c
}
func (d *apiDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var obj types.Object
	resp.Diagnostics.Append(req.Config.Get(ctx, &obj)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, p := range d.spec.Parameters {
		v := obj.Attributes()[snake(p.Name)]
		validateNode(p.Schema, v, path.Root(snake(p.Name)), &resp.Diagnostics)
		if (p.Name == "startIndex" || p.Name == "count") && !v.IsNull() && !v.IsUnknown() && v.(types.Int64).ValueInt64() < 1 {
			resp.Diagnostics.AddAttributeError(path.Root(snake(p.Name)), "Invalid pagination value", "Must be at least 1.")
		}
	}
}
func (d *apiDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var obj types.Object
	resp.Diagnostics.Append(req.Config.Get(ctx, &obj)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Configure the provider before reading data sources.")
		return
	}
	values := obj.Attributes()
	q := url.Values{}
	for _, p := range d.spec.Parameters {
		v := values[snake(p.Name)]
		if v.IsNull() {
			continue
		}
		if v.IsUnknown() {
			resp.Diagnostics.AddError("Unknown request parameter", p.Name+" must be known before reading.")
			return
		}
		if p.In == "path" {
			continue
		}
		switch x := v.(type) {
		case types.String:
			q.Set(p.Name, x.ValueString())
		case types.Int64:
			q.Set(p.Name, strconv.FormatInt(x.ValueInt64(), 10))
		case types.Bool:
			q.Set(p.Name, strconv.FormatBool(x.ValueBool()))
		}
	}
	p, err := route(d.spec.Route, values)
	if err != nil {
		resp.Diagnostics.AddError("Invalid request", err.Error())
		return
	}
	var data any
	if d.spec.List {
		data, err = d.client.List(ctx, p, q)
	} else {
		data, err = d.client.Do(ctx, "GET", p, q, nil)
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Workforce data source", err.Error())
		return
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		resp.Diagnostics.AddError("Invalid response", err.Error())
		return
	}
	values["result_json"] = types.StringValue(string(encoded))
	values["id"] = types.StringValue(fmt.Sprintf("%x", sha256.Sum256([]byte(p+"?"+q.Encode()))))
	state, diags := types.ObjectValue(obj.AttributeTypes(ctx), values)
	resp.Diagnostics.Append(diags...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
