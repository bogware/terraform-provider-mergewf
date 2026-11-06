data "mergewf_scim_service_provider_config" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_scim_service_provider_config.example.result_json)
}
