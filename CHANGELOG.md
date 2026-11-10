# Changelog

## 0.1.0 (November 9, 2026)

- Initial Merge Workforce provider based on HashiCorp Terraform Plugin Framework scaffolding.
- Twelve resources for groups, membership, existing people, tool access, existing skill configuration, skill enablement and person grants, and SCIM provisioning.
- Twenty-six data sources covering every documented Workforce GET endpoint, with automatic cursor and SCIM pagination.
- `mergewf_person` `active` attribute that deactivates or reactivates a person declaratively.
- Opt-in live acceptance tests (`TF_ACC`) and a guide for running them against a tenant.
- Fixture lifecycle tests, generated reference documentation, examples, API coverage checking, CI, and signed release configuration.
