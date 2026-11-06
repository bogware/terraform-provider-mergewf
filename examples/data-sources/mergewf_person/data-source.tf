data "mergewf_person" "example" {
  person_id = "11111111-1111-4111-8111-111111111111"
}

output "result" {
  value = jsondecode(data.mergewf_person.example.result_json)
}
