data "mergewf_group_tool_access_policy" "example" {
  group_id = "11111111-1111-4111-8111-111111111111"
}

output "result" {
  value = jsondecode(data.mergewf_group_tool_access_policy.example.result_json)
}
