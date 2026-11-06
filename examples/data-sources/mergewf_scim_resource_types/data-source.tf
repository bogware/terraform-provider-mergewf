data "mergewf_scim_resource_types" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_scim_resource_types.example.result_json)
}
