data "mergewf_scim_user" "example" {
  user_id = "11111111-1111-4111-8111-111111111111"
}

output "result" {
  value = jsondecode(data.mergewf_scim_user.example.result_json)
}
