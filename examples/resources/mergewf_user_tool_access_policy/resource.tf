resource "mergewf_user_tool_access_policy" "example" {
  user_id = "22222222-2222-4222-8222-222222222222"
  mode    = "CUSTOM"
  rules   = [{ action = "ALLOW", connector = "github", tool_class = "READ_ONLY" }]
}
