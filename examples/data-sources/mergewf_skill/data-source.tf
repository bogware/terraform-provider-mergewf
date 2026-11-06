data "mergewf_skill" "example" {
  slug = "example-skill"
}

output "result" {
  value = jsondecode(data.mergewf_skill.example.result_json)
}
