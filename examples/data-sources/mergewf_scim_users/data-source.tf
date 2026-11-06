data "mergewf_scim_users" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_scim_users.example.result_json)
}
