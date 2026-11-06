data "mergewf_scim_groups" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_scim_groups.example.result_json)
}
