---
page_title: "Lifecycle and ownership"
subcategory: ""
description: |-
  Resource ownership, imports, drift, pagination, and credentials.
---

# Lifecycle and ownership

## Imports

Each resource supports `terraform import`. See its reference or `examples/resources/<type>/import.sh`.

| Resource | Import ID |
| --- | --- |
| Group, tool block, SCIM group, SCIM user | Remote object ID |
| Person | Existing person ID |
| Group member | `group_id/user_id` |
| Group tool policy | Group ID |
| User tool policy | User ID |
| Organization tool policy | `organization` |
| Skill configuration or enablement | Skill slug |
| Skill person grant | `slug/person_id` |

Person and skill configuration resources adopt existing objects when applied or imported. They do not create people or upload bundles. Destroy relinquishes management without deleting the objects. Destroying a person resource does not reactivate a person deactivated through `active = false`. Organization policy removal also leaves the remote policy in place.

## Refresh, drift, and deletion

Refresh reads the remote object, or scans the fully paginated collection when the API has no individual GET endpoint. HTTP 404 and a missing collection member remove the resource from Terraform state. Authorization, transport, validation, and server failures produce diagnostics and retain state.

Group members and skill person grants own a single relationship. Group/user tool access policies own their complete mode and rule list. Deleting these policies restores inherited access. Deleting skill enablement disables the global skill for the organization. SCIM destruction invokes the documented DELETE endpoint; account lifecycle behavior is determined by Merge.

Use one Terraform owner per remote policy or object. Do not combine Terraform SCIM ownership with an identity provider managing the same user, group, or group membership. Avoid managing a group's members through both the SCIM group's `members` collection and individual membership resources.

## Optional attributes and collections

Omitting an optional resource attribute accepts the remote value. This lets imports and server defaults populate state. Omission does not mean deletion. Use an explicit empty list to clear memberships, rules, group IDs, or connector lists; set booleans explicitly to change them. Lists retain API order. Nested optional fields omitted in configuration remain unmanaged rather than absorbing server-only nested defaults.

When a global skill is disabled, the API returns no active reach. The provider retains configured scope and recipient lists locally for the next enable operation. Only enablement status can be refreshed while disabled. Importing a disabled skill cannot recover a previously configured reach; supply it in configuration before enabling.

A person's `active` attribute is derived from the API `status`: only `DEACTIVATED` reads as `false`, so invited people are active. Changing it calls the deactivate or reactivate endpoint. When an apply both changes status and edits name or role, reactivation runs before the edit and deactivation runs after it. Editing a person who stays deactivated sends the edit to a deactivated person. Omit `active` to leave the status unmanaged, and do not set it for people whose `source` is `IDP`; their status belongs to the identity provider.

Tool blocks have no update operation. Attribute changes replace the block. Use `lifecycle { create_before_destroy = true }` to request the new block before deleting the previous one.

## Reading data

Data-source `result_json` is the complete object response or a combined JSON array for list endpoints. Use `jsondecode(data.mergewf_groups.example.result_json)` to obtain the groups. SCIM discovery responses keep their original envelopes. All documented query parameters are available, with camelCase mapped to snake_case and SCIM `count` exposed as `page_size` because Terraform reserves `count`.

Pagination stops at the end of the collection. A repeated cursor, a SCIM page that makes no progress, or more than 10,000 pages returns an error rather than silently returning incomplete data. Use audit-log time filters to keep large queries bounded.

## Credentials and requests

Organization API keys authenticate only `/api/v1` requests; SCIM tokens authenticate only `/scim/v2` requests. Credentials are never put in URLs or logs. Raw error bodies are not included in diagnostics because servers may echo submitted personal data. HTTP status and the operation identify the failing request.

Requests have a 60-second timeout and honor Terraform cancellation. GET requests retry bounded rate-limit and transient gateway failures. Mutations are never automatically retried. Redirects are rejected. No live tenant test is part of the default test suite; see [live acceptance testing](acceptance-testing.md) for the opt-in tests.
