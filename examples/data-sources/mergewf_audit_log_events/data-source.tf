data "mergewf_audit_log_events" "example" {
}

output "result" {
  value = jsondecode(data.mergewf_audit_log_events.example.result_json)
}
