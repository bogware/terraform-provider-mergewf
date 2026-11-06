package provider

import (
	"sort"
	"strings"
	"unicode"

	"github.com/bogware/terraform-provider-mergewf/internal/apispec"
)

var contract = apispec.MustLoad()

type field struct {
	Name, APIName, ReadPath string
	Node                    *apispec.Schema
	Required, Replace, Path bool
}
type resourceSpec struct {
	Name, Description, Route, CreateRoute, CreateMethod, UpdateMethod, DeleteMethod string
	Kind, IDField, ListRoute, MatchField                                            string
	Fields                                                                          []field
	ImportFields                                                                    []string
}

func snake(s string) string {
	// count is reserved by Terraform; SCIM's count parameter is a page size.
	if s == "count" {
		return "page_size"
	}
	var b strings.Builder
	for i, c := range s {
		if unicode.IsUpper(c) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(c))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}
func contains(values []string, s string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func bodyFields(route, method string, replace bool) []field {
	n := contract.Input(route, method)
	result := []field{}
	for name, node := range n.Properties {
		result = append(result, field{Name: snake(name), APIName: name, ReadPath: name, Node: contract.Resolve(node), Required: contains(n.Required, name), Replace: replace})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
func pathField(name string) field {
	return field{Name: name, APIName: name, Node: &apispec.Schema{Type: "string", MinLength: intPtr(1)}, Required: true, Replace: true, Path: true}
}
func intPtr(v int) *int { return &v }
func resources() []resourceSpec {
	group := "/api/v1/groups"
	block := "/api/v1/tool-access/blocks"
	specs := []resourceSpec{
		{Name: "group", Description: "Manages a Workforce group. Import using its group ID. Manage API-created groups separately from groups owned by an identity provider.", Route: group + "/{id}", CreateRoute: group, CreateMethod: "POST", UpdateMethod: "PATCH", DeleteMethod: "DELETE", IDField: "id", Fields: bodyFields(group, "post", false)},
		{Name: "group_member", Description: "Manages one group membership without taking ownership of other members. Import as group_id/user_id.", Route: group + "/{group_id}/members/{user_id}", CreateRoute: group + "/{group_id}/members", CreateMethod: "POST", DeleteMethod: "DELETE", Kind: "member", ListRoute: group + "/{group_id}/members", MatchField: "user_id", ImportFields: []string{"group_id", "user_id"}, Fields: []field{pathField("group_id"), pathField("user_id")}},
		{Name: "tool_access_block", Description: "Manages an organization tool block. Changes replace the block; use create_before_destroy to avoid a gap in enforcement. Import using the block ID.", Route: block + "/{id}", CreateRoute: block, CreateMethod: "POST", DeleteMethod: "DELETE", IDField: "id", ListRoute: block, MatchField: "id", Fields: bodyFields(block, "post", true)},
		{Name: "person", Description: "Manages the name and role of an existing person. person_id must already exist; this resource does not invite or provision a person. Destroy relinquishes management and leaves the person unchanged. Import using the person ID.", Route: "/api/v1/persons/{person_id}", CreateRoute: "/api/v1/persons/{person_id}", CreateMethod: "PATCH", UpdateMethod: "PATCH", Kind: "adopt", ImportFields: []string{"person_id"}, Fields: append([]field{pathField("person_id")}, bodyFields("/api/v1/persons/{person_id}", "patch", false)...)},
	}
	for _, subject := range []string{"group", "user", "organization"} {
		route := "/api/v1/tool-access/organization"
		fields := []field{}
		keys := []string{}
		deleteMethod := ""
		if subject != "organization" {
			key := subject + "_id"
			route = "/api/v1/tool-access/" + subject + "s/{" + key + "}"
			fields = append(fields, pathField(key))
			keys = append(keys, key)
			deleteMethod = "DELETE"
		}
		desc := "Manages the complete " + subject + " tool access policy, including all rules. Import using the subject ID. Removing a group/user policy restores inherited access."
		if subject == "organization" {
			desc = "Manages the complete organization default tool access policy. Import as organization. Destroy relinquishes management and leaves the remote policy unchanged because the API has no delete endpoint."
		}
		specs = append(specs, resourceSpec{Name: subject + "_tool_access_policy", Description: desc, Route: route, CreateRoute: route, CreateMethod: "PUT", UpdateMethod: "PUT", DeleteMethod: deleteMethod, Kind: "policy", ImportFields: keys, Fields: append(fields, bodyFields(route, "put", false)...)})
	}
	for _, kind := range []string{"group", "user"} {
		route := "/scim/v2/" + strings.ToUpper(kind[:1]) + kind[1:] + "s"
		specs = append(specs, resourceSpec{Name: "scim_" + kind, Description: "Provisions a SCIM " + kind + " using the separate SCIM token. Updates use PUT replacement semantics. Do not manage the same object from an identity provider and Terraform. Import using the SCIM ID.", Route: route + "/{id}", CreateRoute: route, CreateMethod: "POST", UpdateMethod: "PUT", DeleteMethod: "DELETE", IDField: "id", Fields: bodyFields(route, "post", false)})
	}
	fields := append([]field{pathField("slug")}, bodyFields("/api/v1/skills/{slug}/enablement", "patch", false)...)
	for i := range fields {
		switch fields[i].Name {
		case "is_enabled":
			fields[i].ReadPath = "enabled"
		case "scope", "group_ids", "person_ids":
			fields[i].ReadPath = "reach." + fields[i].APIName
		}
	}
	specs = append(specs, resourceSpec{Name: "skill_enablement", Description: "Enables or disables a Merge-managed global skill and manages its complete reach. Destroy disables the skill for this organization. Import using its slug.", Route: "/api/v1/skills/{slug}", CreateRoute: "/api/v1/skills/{slug}/enablement", CreateMethod: "PATCH", UpdateMethod: "PATCH", Kind: "enablement", ImportFields: []string{"slug"}, Fields: fields})
	specs = append(specs, resourceSpec{Name: "skill_person_grant", Description: "Pins a live skill to one person in addition to its existing reach. Destroy revokes only this pin. Import as slug/person_id.", Route: "/api/v1/skills/{slug}/people/{person_id}", CreateRoute: "/api/v1/skills/{slug}/people/{person_id}", CreateMethod: "PUT", DeleteMethod: "DELETE", Kind: "grant", ImportFields: []string{"slug", "person_id"}, Fields: []field{pathField("slug"), pathField("person_id")}})
	configuration := append([]field{pathField("slug")}, bodyFields("/api/v1/skills/{slug}", "patch", false)...)
	for i := range configuration {
		switch configuration[i].Name {
		case "access_scope":
			configuration[i].ReadPath = "reach.scope"
		case "group_ids":
			configuration[i].ReadPath = "reach.group_ids"
		}
	}
	specs = append(specs, resourceSpec{Name: "skill_configuration", Description: "Manages display fields, publication, and reach for an existing authored skill. Upload the bundle before adopting it with its slug. Destroy leaves the skill unchanged. Import using its slug. Do not use this resource for Merge-managed global skill enablement.", Route: "/api/v1/skills/{slug}", CreateRoute: "/api/v1/skills/{slug}", CreateMethod: "PATCH", UpdateMethod: "PATCH", Kind: "adopt", ImportFields: []string{"slug"}, Fields: configuration})
	return specs
}

type dataSpec struct {
	Name, Route string
	List        bool
	Parameters  []apispec.Parameter
}

func dataSources() []dataSpec {
	names := map[string]string{
		"/api/v1/persons": "persons", "/api/v1/persons/{person_id}": "person",
		"/api/v1/invites": "invites", "/api/v1/groups": "groups", "/api/v1/groups/{group_id}": "group",
		"/api/v1/groups/{group_id}/managers": "group_managers", "/api/v1/groups/{group_id}/members": "group_members",
		"/api/v1/tool-access/blocks": "tool_access_blocks", "/api/v1/tool-access/groups/{group_id}": "group_tool_access_policy",
		"/api/v1/tool-access/organization": "organization_tool_access_policy", "/api/v1/tool-access/requests": "tool_access_requests",
		"/api/v1/tool-access/requests/{request_id}": "tool_access_request", "/api/v1/tool-access/users/{user_id}": "user_tool_access_policy",
		"/api/v1/tool-access/users/{user_id}/effective": "effective_tool_access", "/api/v1/skills": "skills",
		"/api/v1/skills/publishable-groups": "skill_publishable_groups", "/api/v1/skills/{slug}": "skill",
		"/api/v1/skills/{slug}/versions/{version_id}": "skill_version", "/api/v1/audit-log-events": "audit_log_events",
		"/scim/v2/Groups": "scim_groups", "/scim/v2/Groups/{group_id}": "scim_group", "/scim/v2/Users": "scim_users", "/scim/v2/Users/{user_id}": "scim_user",
		"/scim/v2/ResourceTypes": "scim_resource_types", "/scim/v2/Schemas": "scim_schemas", "/scim/v2/ServiceProviderConfig": "scim_service_provider_config",
	}
	result := []dataSpec{}
	for route, name := range names {
		op := contract.Paths[route]["get"]
		node := contract.Resolve(op.Responses["200"].Content["application/json"].Schema)
		list := node.Kind() == "array" || node.Properties["results"] != nil || name == "scim_groups" || name == "scim_users"
		result = append(result, dataSpec{Name: name, Route: route, List: list, Parameters: op.Parameters})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
