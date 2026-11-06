---
page_title: "API coverage"
subcategory: ""
description: |-
  Implemented operations and deliberate exclusions from the Workforce API.
---

# API coverage

Source: [Merge Workforce OpenAPI](https://docs.merge.dev/openapi/workforce-api.json).

51 implemented operations; 16 deliberate exclusions. Every documented GET endpoint has a data source. The checker also detects changes to parameters and JSON request schemas for implemented operations. The checked-in contract was retrieved on November 5, 2026.

| Operation | Implementation or reason |
| --- | --- |
| `DELETE /api/v1/groups/{group_id}/managers/{person_id}` | Excluded: Requires manage_roles, which organization API keys cannot hold. |
| `DELETE /api/v1/groups/{group_id}/members/{user_id}` | Resource: mergewf_group_member |
| `DELETE /api/v1/groups/{group_id}` | Resource: mergewf_group |
| `DELETE /api/v1/invites/{invite_id}` | Excluded: Invitation workflow is not managed as persistent Terraform state. |
| `DELETE /api/v1/skills/{slug}/people/{person_id}` | Resource: mergewf_skill_person_grant |
| `DELETE /api/v1/skills/{slug}/versions/{version_id}` | Excluded: Versions are not owned without a documented upload contract. |
| `DELETE /api/v1/skills/{slug}` | Excluded: Skill configuration adopts an existing upload and relinquishes management on destroy; it does not own the bundle. |
| `DELETE /api/v1/tool-access/blocks/{block_id}` | Resource: mergewf_tool_access_block |
| `DELETE /api/v1/tool-access/groups/{group_id}` | Resource: mergewf_group_tool_access_policy |
| `DELETE /api/v1/tool-access/users/{user_id}` | Resource: mergewf_user_tool_access_policy |
| `DELETE /scim/v2/Groups/{group_id}` | Resource: mergewf_scim_group |
| `DELETE /scim/v2/Users/{user_id}` | Resource: mergewf_scim_user |
| `GET /api/v1/audit-log-events` | Data source: mergewf_audit_log_events |
| `GET /api/v1/groups/{group_id}/managers` | Data source: mergewf_group_managers |
| `GET /api/v1/groups/{group_id}/members` | Data source: mergewf_group_members |
| `GET /api/v1/groups/{group_id}` | Data source: mergewf_group |
| `GET /api/v1/groups` | Data source: mergewf_groups |
| `GET /api/v1/invites` | Data source: mergewf_invites |
| `GET /api/v1/persons/{person_id}` | Data source: mergewf_person |
| `GET /api/v1/persons` | Data source: mergewf_persons |
| `GET /api/v1/skills/publishable-groups` | Data source: mergewf_skill_publishable_groups |
| `GET /api/v1/skills/{slug}/versions/{version_id}` | Data source: mergewf_skill_version |
| `GET /api/v1/skills/{slug}` | Data source: mergewf_skill |
| `GET /api/v1/skills` | Data source: mergewf_skills |
| `GET /api/v1/tool-access/blocks` | Data source: mergewf_tool_access_blocks |
| `GET /api/v1/tool-access/groups/{group_id}` | Data source: mergewf_group_tool_access_policy |
| `GET /api/v1/tool-access/organization` | Data source: mergewf_organization_tool_access_policy |
| `GET /api/v1/tool-access/requests/{request_id}` | Data source: mergewf_tool_access_request |
| `GET /api/v1/tool-access/requests` | Data source: mergewf_tool_access_requests |
| `GET /api/v1/tool-access/users/{user_id}/effective` | Data source: mergewf_effective_tool_access |
| `GET /api/v1/tool-access/users/{user_id}` | Data source: mergewf_user_tool_access_policy |
| `GET /scim/v2/Groups/{group_id}` | Data source: mergewf_scim_group |
| `GET /scim/v2/Groups` | Data source: mergewf_scim_groups |
| `GET /scim/v2/ResourceTypes` | Data source: mergewf_scim_resource_types |
| `GET /scim/v2/Schemas` | Data source: mergewf_scim_schemas |
| `GET /scim/v2/ServiceProviderConfig` | Data source: mergewf_scim_service_provider_config |
| `GET /scim/v2/Users/{user_id}` | Data source: mergewf_scim_user |
| `GET /scim/v2/Users` | Data source: mergewf_scim_users |
| `PATCH /api/v1/groups/{group_id}` | Resource: mergewf_group |
| `PATCH /api/v1/persons/{person_id}` | Resource: mergewf_person |
| `PATCH /api/v1/skills/{slug}/enablement` | Resource: mergewf_skill_enablement |
| `PATCH /api/v1/skills/{slug}` | Resource: mergewf_skill_configuration |
| `PATCH /scim/v2/Groups/{group_id}` | Excluded: The SCIM group resource uses PUT replacement instead of incremental PATCH operations. |
| `PATCH /scim/v2/Users/{user_id}` | Excluded: The SCIM user resource uses PUT replacement instead of incremental PATCH operations. |
| `POST /api/v1/groups/{group_id}/managers` | Excluded: Requires manage_roles, which organization API keys cannot hold. |
| `POST /api/v1/groups/{group_id}/members` | Resource: mergewf_group_member |
| `POST /api/v1/groups` | Resource: mergewf_group |
| `POST /api/v1/invites/{invite_id}/resend` | Excluded: Resending email is an imperative action. |
| `POST /api/v1/invites` | Excluded: Sending invitations is a transient workflow; accepted or expired invitations should not be recreated by refresh. |
| `POST /api/v1/persons/{person_id}/deactivate` | Resource: mergewf_person |
| `POST /api/v1/persons/{person_id}/reactivate` | Resource: mergewf_person |
| `POST /api/v1/skills/{slug}/review` | Excluded: Review decisions are an imperative workflow. |
| `POST /api/v1/skills/{slug}/versions/{version_id}/promote` | Excluded: Version promotion is an imperative workflow. |
| `POST /api/v1/skills/{slug}/versions` | Excluded: The published reference omits the version-upload request contract. |
| `POST /api/v1/skills` | Excluded: The published OpenAPI and endpoint reference omit multipart field names and schemas; do not guess an upload contract. |
| `POST /api/v1/tool-access/blocks` | Resource: mergewf_tool_access_block |
| `POST /api/v1/tool-access/requests/{request_id}/approve` | Excluded: Access-request review is an imperative workflow; manage durable grants through tool access policies. |
| `POST /api/v1/tool-access/requests/{request_id}/deny` | Excluded: Access-request review is an imperative workflow; manage durable grants through tool access policies. |
| `POST /api/v1/tool-access/requests/{request_id}/revoke` | Excluded: Access-request review is an imperative workflow; manage durable grants through tool access policies. |
| `POST /scim/v2/Groups` | Resource: mergewf_scim_group |
| `POST /scim/v2/Users` | Resource: mergewf_scim_user |
| `PUT /api/v1/skills/{slug}/people/{person_id}` | Resource: mergewf_skill_person_grant |
| `PUT /api/v1/tool-access/groups/{group_id}` | Resource: mergewf_group_tool_access_policy |
| `PUT /api/v1/tool-access/organization` | Resource: mergewf_organization_tool_access_policy |
| `PUT /api/v1/tool-access/users/{user_id}` | Resource: mergewf_user_tool_access_policy |
| `PUT /scim/v2/Groups/{group_id}` | Resource: mergewf_scim_group |
| `PUT /scim/v2/Users/{user_id}` | Resource: mergewf_scim_user |
