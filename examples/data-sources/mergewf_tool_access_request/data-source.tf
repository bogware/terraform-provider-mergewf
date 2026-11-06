data "mergewf_tool_access_request" "example" {
  request_id = "11111111-1111-4111-8111-111111111111"
}

output "result" {
  value = jsondecode(data.mergewf_tool_access_request.example.result_json)
}
