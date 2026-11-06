data "mergewf_skill_publishable_groups" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_skill_publishable_groups.example.result_json)
}
