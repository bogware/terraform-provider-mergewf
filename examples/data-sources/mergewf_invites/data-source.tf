data "mergewf_invites" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_invites.example.result_json)
}
