data "mergewf_persons" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_persons.example.result_json)
}
