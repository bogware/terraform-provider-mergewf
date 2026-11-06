package provider

import (
	"regexp"
	"strings"
)

var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

func OperationKey(method, route string) string {
	return strings.ToUpper(method) + " " + pathParameter.ReplaceAllString(route, "{}")
}

// APICoverage is an auditable account of every published operation. Exclusions
// are deliberate lifecycle or documented API limitations, not invented routes.
func APICoverage() map[string]string {
	result := map[string]string{}
	for _, s := range dataSources() {
		result[OperationKey("GET", s.Route)] = "Data source: mergewf_" + s.Name
	}
	for _, s := range resources() {
		result[OperationKey(s.CreateMethod, s.CreateRoute)] = "Resource: mergewf_" + s.Name
		if s.UpdateMethod != "" {
			route := s.Route
			if s.Kind == "enablement" {
				route = s.CreateRoute
			}
			result[OperationKey(s.UpdateMethod, route)] = "Resource: mergewf_" + s.Name
		}
		if s.DeleteMethod != "" {
			result[OperationKey(s.DeleteMethod, s.Route)] = "Resource: mergewf_" + s.Name
		}
	}
	exclude := func(method, route, reason string) { result[OperationKey(method, route)] = "Excluded: " + reason }
	// The person resource applies its active attribute through these commands.
	for _, command := range []string{"deactivate", "reactivate"} {
		result[OperationKey("POST", "/api/v1/persons/{person_id}/"+command)] = "Resource: mergewf_person"
	}
	exclude("POST", "/api/v1/invites", "Sending invitations is a transient workflow; accepted or expired invitations should not be recreated by refresh.")
	exclude("DELETE", "/api/v1/invites/{invite_id}", "Invitation workflow is not managed as persistent Terraform state.")
	exclude("POST", "/api/v1/invites/{invite_id}/resend", "Resending email is an imperative action.")
	exclude("POST", "/api/v1/groups/{group_id}/managers", "Requires manage_roles, which organization API keys cannot hold.")
	exclude("DELETE", "/api/v1/groups/{group_id}/managers/{person_id}", "Requires manage_roles, which organization API keys cannot hold.")
	for _, decision := range []string{"approve", "deny", "revoke"} {
		exclude("POST", "/api/v1/tool-access/requests/{request_id}/"+decision, "Access-request review is an imperative workflow; manage durable grants through tool access policies.")
	}
	exclude("POST", "/api/v1/skills", "The published OpenAPI and endpoint reference omit multipart field names and schemas; do not guess an upload contract.")
	exclude("DELETE", "/api/v1/skills/{slug}", "Skill configuration adopts an existing upload and relinquishes management on destroy; it does not own the bundle.")
	exclude("POST", "/api/v1/skills/{slug}/review", "Review decisions are an imperative workflow.")
	exclude("POST", "/api/v1/skills/{slug}/versions", "The published reference omits the version-upload request contract.")
	exclude("DELETE", "/api/v1/skills/{slug}/versions/{version_id}", "Versions are not owned without a documented upload contract.")
	exclude("POST", "/api/v1/skills/{slug}/versions/{version_id}/promote", "Version promotion is an imperative workflow.")
	exclude("PATCH", "/scim/v2/Groups/{group_id}", "The SCIM group resource uses PUT replacement instead of incremental PATCH operations.")
	exclude("PATCH", "/scim/v2/Users/{user_id}", "The SCIM user resource uses PUT replacement instead of incremental PATCH operations.")
	return result
}
