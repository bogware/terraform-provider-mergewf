data "mergewf_skill_version" "example" {
  slug       = "example-skill"
  version_id = "11111111-1111-4111-8111-111111111111"
}

output "result" {
  value = jsondecode(data.mergewf_skill_version.example.result_json)
}
