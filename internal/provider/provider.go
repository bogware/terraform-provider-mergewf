// Copyright (c) Bogware
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/bogware/terraform-provider-mergewf/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = &WorkforceProvider{}

type WorkforceProvider struct{ version string }
type providerModel struct {
	APIKey    types.String `tfsdk:"api_key"`
	SCIMToken types.String `tfsdk:"scim_token"`
	BaseURL   types.String `tfsdk:"base_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &WorkforceProvider{version: version} }
}
func (p *WorkforceProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "mergewf"
	resp.Version = p.version
}
func (p *WorkforceProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{MarkdownDescription: "Manage Merge Workforce groups, people, tool access, skill access, and SCIM provisioning. Workforce organization keys and SCIM tokens are separate credentials.", Attributes: map[string]schema.Attribute{
		"api_key":    schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Organization API key. Defaults to MERGEWF_API_KEY. Required for /api/v1 operations; keys need both read and write scopes for managed resources."},
		"scim_token": schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "SCIM provisioning token. Defaults to MERGEWF_SCIM_TOKEN. Required for SCIM resources and data sources."},
		"base_url":   schema.StringAttribute{Optional: true, MarkdownDescription: "Workforce API origin. Defaults to MERGEWF_BASE_URL or https://wf-api.merge.dev. HTTP is allowed only for loopback fixture servers."},
	}}
}
func (p *WorkforceProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, value := range []types.String{config.APIKey, config.SCIMToken, config.BaseURL} {
		if value.IsUnknown() {
			resp.Diagnostics.AddError("Unknown provider configuration", "Provider credentials and base_url must be known before planning.")
			return
		}
	}
	resolve := func(v types.String, env string) string {
		if !v.IsNull() {
			return v.ValueString()
		}
		return os.Getenv(env)
	}
	base := resolve(config.BaseURL, "MERGEWF_BASE_URL")
	if base == "" {
		base = client.DefaultBaseURL
	}
	c, err := client.New(base, resolve(config.APIKey, "MERGEWF_API_KEY"), resolve(config.SCIMToken, "MERGEWF_SCIM_TOKEN"), p.version)
	if err != nil {
		resp.Diagnostics.AddError("Invalid provider configuration", err.Error())
		return
	}
	resp.ResourceData = c
	resp.DataSourceData = c
}
func (p *WorkforceProvider) Resources(context.Context) []func() resource.Resource {
	result := []func() resource.Resource{}
	for _, spec := range resources() {
		result = append(result, func() resource.Resource { return &apiResource{spec: spec} })
	}
	return result
}
func (p *WorkforceProvider) DataSources(context.Context) []func() datasource.DataSource {
	result := []func() datasource.DataSource{}
	for _, spec := range dataSources() {
		result = append(result, func() datasource.DataSource { return &apiDataSource{spec: spec} })
	}
	return result
}
