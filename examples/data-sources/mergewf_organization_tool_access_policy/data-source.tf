data "mergewf_organization_tool_access_policy" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_organization_tool_access_policy.example.result_json)
}
