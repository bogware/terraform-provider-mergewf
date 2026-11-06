data "mergewf_tool_access_requests" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_tool_access_requests.example.result_json)
}
