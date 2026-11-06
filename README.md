# Terraform Provider for Merge Workforce

Manage [Merge Workforce](https://docs.merge.dev/merge-workforce/api) with Terraform. The provider source address is `bogware/mergewf`; its Go module is `github.com/bogware/terraform-provider-mergewf`.

Built from the HashiCorp Terraform Plugin Framework scaffolding, using Plugin Framework v1.19.0 and Go 1.25.8 or newer. This initial implementation is validated against local HTTP fixtures and the published API contract. It has not been tested against a live Workforce tenant or released to the Terraform Registry.

## Configuration

```hcl
terraform {
  required_providers {
    mergewf = {
      source = "bogware/mergewf"
    }
  }
}

# Supply credentials through the environment.
provider "mergewf" {}

resource "mergewf_group" "engineering" {
  name = "Engineering"
}

resource "mergewf_group_tool_access_policy" "engineering" {
  group_id = mergewf_group.engineering.id
  mode     = "CUSTOM"
  rules = [{
    action     = "ALLOW"
    connector  = "github"
    tool_class = "READ_ONLY"
  }]
}
```

Set `MERGEWF_API_KEY` to a Workforce organization API key (`mfw_org_key_...`). SCIM resources require the separate `MERGEWF_SCIM_TOKEN`. These can also be set as sensitive provider attributes `api_key` and `scim_token`. An explicitly configured value takes precedence over its environment variable. Supply only the credential needed by the resources you use.

The default API origin is `https://wf-api.merge.dev`; override it with `base_url` or `MERGEWF_BASE_URL`. HTTPS is required except for loopback test servers. Resource management needs the matching read and write permission scopes.

## Resources and data sources

There are **12 resources**:

| Area | Resources |
| --- | --- |
| Groups | `mergewf_group`, `mergewf_group_member` |
| Existing people | `mergewf_person` |
| Tool access | `mergewf_tool_access_block`, `mergewf_group_tool_access_policy`, `mergewf_user_tool_access_policy`, `mergewf_organization_tool_access_policy` |
| Skills | `mergewf_skill_configuration`, `mergewf_skill_enablement`, `mergewf_skill_person_grant` |
| SCIM | `mergewf_scim_group`, `mergewf_scim_user` |

All **26 documented GET endpoints** have data sources, including people, invitations, groups, managers, members, tool policies, requests, effective access, skills, skill versions, audit events, and SCIM discovery. They expose each documented filter and return `result_json`. For list endpoints this is a JSON array combining all pages; use `jsondecode()` to consume it. `limit` and SCIM `page_size` control page size, not total results. SCIM discovery endpoints retain their response envelopes.

Resources expose typed configuration attributes and return additional API fields in `result_json`. Omitted optional resource attributes accept the server value. Supply explicit empty lists to clear collections. API responses, including personal data, are stored in Terraform state; protect the state backend.

See [resource reference](docs/resources), [data source reference](docs/data-sources), [examples](examples), [lifecycle behavior](docs/guides/lifecycle.md), and the complete [API coverage report](docs/guides/api-coverage.md).

## Lifecycle boundaries

- `mergewf_person` edits an existing person; use SCIM to provision identities. Destroy leaves the person unchanged.
- `mergewf_skill_configuration` edits an existing authored skill. Upload its bundle first; destroy leaves the skill unchanged.
- Group and user tool policies own all rules for their subject. Destroy deletes the policy and restores inherited access. Organization policy destroy leaves the remote policy unchanged because there is no delete endpoint.
- Group membership and skill person grants manage one relationship each, without owning other relationships.
- Tool block changes require replacement. Use `create_before_destroy` to avoid removing a block before its replacement exists.
- Do not manage an identity-provider-owned SCIM object concurrently from Terraform.

Group manager writes require `manage_roles`, which organization API keys cannot hold. Skill upload/version request fields are absent from the published API contract, so the provider does not guess their multipart format. Invitations, reviews, and other one-time commands are deliberately excluded. The coverage report lists every exclusion and its reason.

## Build and test

Install Go >= 1.25.8 and Terraform >= 1.13, then run:

```text
go build ./...
go test ./...
go vet ./...
go run ./cmd/check-api-coverage
go -C tools generate -tags generate .
```

`go test ./...` runs the real Terraform CLI against local HTTP fixtures. It needs no Merge credentials and performs no live Merge operations. The API coverage checker downloads only the public specification; `-offline` checks the committed snapshot. Generation creates examples, exports protocol schemas directly (including on Windows), and runs tfplugindocs from the scaffolding's separate tools module.

Build a local plugin on Windows:

```powershell
go build -o bin/terraform-provider-mergewf.exe .
```

Put this in your Terraform CLI configuration (`%APPDATA%/terraform.rc` on Windows), adjusting the path if necessary:

```hcl
provider_installation {
  dev_overrides {
    "bogware/mergewf" = "C:/BogWare/terraform-provider-mergewf/bin"
  }
  direct {}
}
```

With the override active, use `terraform plan` and `terraform apply` from your configuration directory. `terraform init` still attempts to resolve a published provider, so skip it for provider-only development configurations before the first Registry release. Applying real configurations changes your tenant; the test suite uses only local fixtures.

## Releases

The scaffolding's GoReleaser v2 configuration builds signed release archives. Pushing a `v*` tag runs the release workflow; it needs repository secrets `GPG_PRIVATE_KEY` and `PASSPHRASE`. Update the changelog and run tests, generation, and API coverage before tagging. Register `bogware/mergewf` with the Terraform Registry separately. No release or Registry registration has been performed.
