data "mergewf_effective_tool_access" "example" {
  user_id = "11111111-1111-4111-8111-111111111111"
}

output "result" {
  value = jsondecode(data.mergewf_effective_tool_access.example.result_json)
}
