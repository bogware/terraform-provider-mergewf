---
page_title: "Live acceptance testing"
subcategory: ""
description: |-
  Running the acceptance tests against a real Workforce tenant, and what to verify by hand.
---

# Live acceptance testing

The default `go test ./...` suite uses local HTTP fixtures only. The `TestAcc*` tests in `internal/provider/acceptance_test.go` run against a real tenant and **change it**. They are skipped unless `TF_ACC` is set. Use a sandbox tenant if you have one.

## What the tests do

| Test | Needs | Effect on the tenant |
| --- | --- | --- |
| `TestAccReadOnlyDataSources` | `MERGEWF_API_KEY` (plus `MERGEWF_SCIM_TOKEN` for SCIM reads) | Read only. Exercises pagination and response decoding. |
| `TestAccGroupMembershipAndPolicy` | `MERGEWF_API_KEY`; optional `MERGEWF_ACC_USER_ID` | Creates a `tf-acc-*` group with a custom tool policy (and one member if a user ID is set), renames it, changes the policy, imports each resource, then deletes everything. |
| `TestAccPersonActive` | `MERGEWF_API_KEY`, `MERGEWF_ACC_PERSON_ID` | Deactivates and then reactivates one person. The test always ends reactivated and fails loudly if the person is left deactivated. |

The tests never touch the organization tool policy or tool blocks, because those apply to the whole organization and the organization policy has no delete endpoint.

For `MERGEWF_ACC_PERSON_ID`, use a disposable, manually created person (source `MANUAL` or `API`), not someone provisioned by your identity provider and not your own account. IDs must be real UUIDs.

## Running

The API key needs read and write permissions for groups, tool access and people (`manage_users`).

```powershell
$env:TF_ACC = '1'
$env:MERGEWF_API_KEY = 'mfw_org_key_...'
$env:MERGEWF_ACC_USER_ID = '<existing user UUID>'      # optional
$env:MERGEWF_ACC_PERSON_ID = '<disposable person UUID>' # optional
go test ./internal/provider -run TestAcc -count=1 -v -timeout 30m
```

On macOS or Linux, use `make testacc` with the same variables exported.

Start with `-run TestAccReadOnlyDataSources`. It changes nothing and catches most response-shape problems.

## Cleanup

If a run is interrupted, look for groups named `tf-acc-*` and delete them. If `TestAccPersonActive` fails partway through, check that the test person is active again.

## Things to check by hand

These are behaviors the fixtures cannot prove. Record what you find; any surprise is a bug to fix in the provider.

- **Deleted objects return 404.** Refresh removes state only on HTTP 404. `TestAccGroupMembershipAndPolicy` checks this for groups in `CheckDestroy`. If the API returns 403 or 422 for a missing object, out-of-band deletions will produce errors instead of a plan to recreate.
- **No diff after apply.** The `PlanOnly` steps fail if the API normalizes a value differently from configuration, such as case, ordering of rules, or `null` versus a missing field.
- **Pagination.** Run the read-only test against a tenant with more people than one page to confirm `next_cursor` handling.
- **Person status.** Confirm that deactivating an `INVITED` person is allowed, and that the person's `status` in `result_json` is `ACTIVE` after reactivation.
- **Editing a deactivated person.** With `active = false` already applied, change `name` and apply. Confirm the API accepts the PATCH for a person who stays deactivated.
- **Connector names.** The group policy uses connector `github` with tool class `READ_ONLY`. If your tenant rejects that connector, change it in the test.
- **Permission errors.** Run once with a read-only key and confirm that writes fail with a clear 403 diagnostic and leave state intact.
