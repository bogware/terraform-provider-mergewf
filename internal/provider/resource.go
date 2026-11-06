package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/bogware/terraform-provider-mergewf/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &apiResource{}
var _ resource.ResourceWithConfigure = &apiResource{}
var _ resource.ResourceWithImportState = &apiResource{}
var _ resource.ResourceWithValidateConfig = &apiResource{}
var _ resource.ResourceWithModifyPlan = &apiResource{}

type apiResource struct {
	spec   resourceSpec
	client *client.Client
}

func (r *apiResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var planned, previous types.Object
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planned)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &previous)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Framework marks computed values unknown before attribute modifiers run.
	// If those modifiers preserve omitted fields and leave no actual changes,
	// the response is already current from refresh; avoid a computed-only diff.
	old := previous.Attributes()
	for name, value := range planned.Attributes() {
		if name != "result_json" && !value.Equal(old[name]) {
			return
		}
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("result_json"), old["result_json"])...)
}

func (r *apiResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.spec.Name
}
func (r *apiResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Terraform resource identity. See the resource description for its import format.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"result_json": schema.StringAttribute{Computed: true, MarkdownDescription: "Complete API response encoded as JSON. Use jsondecode to access additional response fields. Stored in Terraform state."},
	}
	for _, f := range r.spec.Fields {
		description := "API field `" + f.APIName + "`."
		if f.Virtual {
			description = "Derived from API field `" + f.ReadPath + "`."
		} else if !f.Required {
			description += " Omit to accept the server value; use an explicit empty collection to clear a collection."
		}
		attrs[f.Name] = resourceAttribute(f.Node, f.Required, !f.Required, f.Replace, description)
	}
	resp.Schema = schema.Schema{MarkdownDescription: r.spec.Description, Attributes: attrs}
}
func (r *apiResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Provider configuration error", "Unexpected client type.")
		return
	}
	r.client = c
}
func (r *apiResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var obj types.Object
	resp.Diagnostics.Append(req.Config.Get(ctx, &obj)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for _, f := range r.spec.Fields {
		validateNode(f.Node, obj.Attributes()[f.Name], path.Root(f.Name), &resp.Diagnostics)
	}
	if r.spec.Name == "tool_access_block" {
		known := true
		count := 0
		for _, k := range []string{"connector", "tool", "tool_class"} {
			v := obj.Attributes()[k]
			if v.IsUnknown() {
				known = false
			}
			if !v.IsNull() && !v.IsUnknown() {
				count++
			}
		}
		if known && count == 0 {
			resp.Diagnostics.AddError("Missing block selector", "Set at least one of connector, tool, or tool_class.")
		}
	}
}
func textValue(v attr.Value) string {
	if value, ok := v.(types.String); ok {
		return value.ValueString()
	}
	return ""
}
func route(template string, values map[string]attr.Value) (string, error) {
	for k, v := range values {
		placeholder := "{" + k + "}"
		if strings.Contains(template, placeholder) {
			s := textValue(v)
			if s == "" || s == "." || s == ".." {
				return "", fmt.Errorf("%s must be a nonempty identifier other than . or ..", k)
			}
			template = strings.ReplaceAll(template, placeholder, url.PathEscape(s))
		}
	}
	if strings.ContainsAny(template, "{}") {
		return "", fmt.Errorf("missing path identifier for %s", template)
	}
	return template, nil
}
func lookup(obj map[string]any, key string) any {
	var current any = obj
	for _, part := range strings.Split(key, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}
func (r *apiResource) body(values map[string]attr.Value) map[string]any {
	body := map[string]any{}
	for _, f := range r.spec.Fields {
		if f.Path || f.Virtual {
			continue
		}
		v := values[f.Name]
		if v != nil && !v.IsNull() && !v.IsUnknown() {
			body[f.APIName] = native(f.Node, v)
		}
	}
	if r.spec.Kind == "member" {
		body["user_ids"] = []string{textValue(values["user_id"])}
	}
	return body
}
func (r *apiResource) identity(values map[string]attr.Value, obj map[string]any) (string, error) {
	if r.spec.IDField != "" {
		id, ok := obj[r.spec.IDField].(string)
		if !ok || id == "" {
			return "", fmt.Errorf("API response is missing the resource identifier")
		}
		return id, nil
	}
	if len(r.spec.ImportFields) == 0 {
		return "organization", nil
	}
	parts := []string{}
	for _, k := range r.spec.ImportFields {
		parts = append(parts, textValue(values[k]))
	}
	return strings.Join(parts, "/"), nil
}
func (r *apiResource) fetch(ctx context.Context, values map[string]attr.Value) (map[string]any, error) {
	template := r.spec.Route
	if r.spec.ListRoute != "" {
		template = r.spec.ListRoute
	}
	if r.spec.Kind == "grant" {
		template = "/api/v1/skills/{slug}"
	}
	p, err := route(template, values)
	if err != nil {
		return nil, err
	}
	if r.spec.ListRoute != "" {
		items, err := r.client.List(ctx, p, nil)
		if err != nil {
			return nil, err
		}
		key := r.spec.MatchField
		wanted := textValue(values["id"])
		if r.spec.Kind == "member" {
			wanted = textValue(values["user_id"])
		}
		for _, item := range items {
			obj, ok := item.(map[string]any)
			if ok && obj[key] == wanted {
				return obj, nil
			}
		}
		return nil, &client.APIError{Status: 404, Method: "GET", Path: p}
	}
	data, err := r.client.Do(ctx, "GET", p, nil, nil)
	if err != nil {
		return nil, err
	}
	obj, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object response for %s", p)
	}
	if r.spec.Kind == "grant" {
		pins, _ := lookup(obj, "reach.pinned_person_ids").([]any)
		found := false
		for _, pin := range pins {
			if pin == textValue(values["person_id"]) {
				found = true
			}
		}
		if !found {
			return nil, &client.APIError{Status: 404, Method: "GET", Path: p}
		}
	}
	return obj, nil
}
func (r *apiResource) save(ctx context.Context, state *tfsdk.State, prior types.Object, obj map[string]any, d *diag.Diagnostics) {
	values := prior.Attributes()
	for _, f := range r.spec.Fields {
		if f.Path {
			continue
		}
		data := lookup(obj, f.ReadPath)
		// A disabled global skill has no remote reach to refresh. Keep the
		// configured reach locally so enabling it again sends the desired scope.
		if r.spec.Kind == "enablement" && f.Name != "is_enabled" && lookup(obj, "reach.scope") == nil {
			continue
		}
		if r.spec.Kind == "enablement" && f.Name == "is_enabled" {
			data = lookup(obj, "reach.scope") != nil
		}
		if f.Name == "active" && f.Virtual {
			if status, ok := data.(string); ok {
				data = status != "DEACTIVATED"
			}
		}
		v, err := terraformValue(ctx, f.Node, data, values[f.Name])
		if err != nil {
			d.AddError("Invalid API response", f.Name+": "+err.Error())
			return
		}
		values[f.Name] = v
	}
	encoded, err := json.Marshal(obj)
	if err != nil {
		d.AddError("Invalid API response", err.Error())
		return
	}
	values["result_json"] = types.StringValue(string(encoded))
	object, diags := types.ObjectValue(prior.AttributeTypes(ctx), values)
	d.Append(diags...)
	if !d.HasError() {
		d.Append(state.Set(ctx, object)...)
	}
}
func (r *apiResource) write(ctx context.Context, plan types.Object, create bool, state *tfsdk.State, d *diag.Diagnostics) {
	if r.client == nil {
		d.AddError("Unconfigured provider", "Configure the provider before using resources.")
		return
	}
	values := plan.Attributes()
	template := r.spec.Route
	method := r.spec.UpdateMethod
	if create || r.spec.Kind == "enablement" {
		template = r.spec.CreateRoute
	}
	if create {
		method = r.spec.CreateMethod
	}
	if method == "" {
		d.AddError("Unsupported update", "This resource must be replaced to change its attributes.")
		return
	}
	p, err := route(template, values)
	if err != nil {
		d.AddError("Invalid resource identifier", err.Error())
		return
	}
	var body any = r.body(values)
	if r.spec.Kind == "grant" {
		body = nil
	}
	active, manageStatus := values["active"].(types.Bool)
	// Only the person resource's virtual active field maps to status commands;
	// SCIM users send active in their request body.
	manageStatus = manageStatus && r.spec.Name == "person" && !active.IsNull() && !active.IsUnknown()
	deactivated := false
	if manageStatus {
		// Reactivate before editing and deactivate after editing, so the
		// PATCH never targets a deactivated person.
		current, err := r.client.Do(ctx, "GET", p, nil, nil)
		if err != nil {
			d.AddError("Unable to read person status", err.Error())
			return
		}
		currentObj, _ := current.(map[string]any)
		deactivated = currentObj["status"] == "DEACTIVATED"
		if active.ValueBool() && deactivated {
			if _, err := r.client.Do(ctx, "POST", p+"/reactivate", nil, nil); err != nil {
				d.AddError("Unable to reactivate person", err.Error())
				return
			}
		}
	}
	data, err := r.client.Do(ctx, method, p, nil, body)
	if err != nil {
		d.AddError("Unable to write Workforce resource", err.Error())
		return
	}
	if manageStatus && !active.ValueBool() && !deactivated {
		data, err = r.client.Do(ctx, "POST", p+"/deactivate", nil, nil)
		if err != nil {
			d.AddError("Unable to deactivate person", err.Error())
			return
		}
	}
	obj, _ := data.(map[string]any)
	if obj == nil {
		obj = map[string]any{}
	}
	if create {
		id, err := r.identity(values, obj)
		if err != nil {
			d.AddError("Missing resource identity", err.Error())
			return
		}
		values["id"] = types.StringValue(id)
	}
	// Save a known identity immediately so a subsequent read failure cannot orphan
	// a successfully created membership or grant.
	for k, v := range values {
		if v.IsUnknown() {
			t := v.Type(ctx)
			known, _ := t.ValueFromTerraform(ctx, nullTerraform(ctx, t))
			values[k] = known
		}
	}
	values["result_json"] = types.StringValue("{}")
	known, diags := types.ObjectValue(plan.AttributeTypes(ctx), values)
	d.Append(diags...)
	d.Append(state.Set(ctx, known)...)
	if d.HasError() {
		return
	}
	if r.spec.Kind == "member" || r.spec.Kind == "grant" {
		obj, err = r.fetch(ctx, values)
		if err != nil {
			d.AddError("Unable to refresh created resource", err.Error())
			return
		}
	}
	r.save(ctx, state, known, obj, d)
}
func (r *apiResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var obj types.Object
	resp.Diagnostics.Append(req.Plan.Get(ctx, &obj)...)
	if !resp.Diagnostics.HasError() {
		r.write(ctx, obj, true, &resp.State, &resp.Diagnostics)
	}
}
func (r *apiResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var obj types.Object
	resp.Diagnostics.Append(req.Plan.Get(ctx, &obj)...)
	var previous types.Object
	resp.Diagnostics.Append(req.State.Get(ctx, &previous)...)
	if !resp.Diagnostics.HasError() {
		// Optional/computed fields may become unknown when another field changes.
		// Preserve their known state in replacement PUT requests; omitting them
		// could otherwise clear unmanaged memberships or policy rules.
		values := obj.Attributes()
		for key, value := range values {
			if value.IsUnknown() {
				values[key] = previous.Attributes()[key]
			}
		}
		var diags diag.Diagnostics
		obj, diags = types.ObjectValue(obj.AttributeTypes(ctx), values)
		resp.Diagnostics.Append(diags...)
	}
	if !resp.Diagnostics.HasError() {
		r.write(ctx, obj, false, &resp.State, &resp.Diagnostics)
	}
}
func (r *apiResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var prior types.Object
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Configure the provider before reading resources.")
		return
	}
	obj, err := r.fetch(ctx, prior.Attributes())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Workforce resource", err.Error())
		return
	}
	r.save(ctx, &resp.State, prior, obj, &resp.Diagnostics)
}
func (r *apiResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.spec.DeleteMethod == "" && r.spec.Kind != "enablement" {
		return
	}
	var obj types.Object
	resp.Diagnostics.Append(req.State.Get(ctx, &obj)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.client == nil {
		resp.Diagnostics.AddError("Unconfigured provider", "Configure the provider before deleting resources.")
		return
	}
	template := r.spec.Route
	method := r.spec.DeleteMethod
	var body any
	if r.spec.Kind == "enablement" {
		template = r.spec.CreateRoute
		method = "PATCH"
		body = map[string]any{"is_enabled": false}
	}
	p, err := route(template, obj.Attributes())
	if err == nil {
		_, err = r.client.Do(ctx, method, p, nil, body)
	}
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete Workforce resource", err.Error())
	}
}
func (r *apiResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	keys := r.spec.ImportFields
	parts := strings.Split(req.ID, "/")
	if len(keys) == 0 {
		keys = []string{"id"}
	}
	if len(parts) != len(keys) {
		resp.Diagnostics.AddError("Invalid import ID", "Expected "+strings.Join(keys, "/")+".")
		return
	}
	for i, k := range keys {
		if parts[i] == "" || parts[i] == "." || parts[i] == ".." {
			resp.Diagnostics.AddError("Invalid import ID", "Import identifiers must not be empty, . or ..")
			return
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(k), parts[i])...)
	}
	if r.spec.Name == "organization_tool_access_policy" && req.ID != "organization" {
		resp.Diagnostics.AddError("Invalid import ID", "Use organization as the import ID.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
