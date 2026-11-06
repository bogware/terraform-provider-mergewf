data "mergewf_scim_schemas" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_scim_schemas.example.result_json)
}
