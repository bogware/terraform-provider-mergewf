data "mergewf_tool_access_blocks" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_tool_access_blocks.example.result_json)
}
