data "mergewf_skills" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_skills.example.result_json)
}
