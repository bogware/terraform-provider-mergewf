data "mergewf_groups" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_groups.example.result_json)
}
