resource "mergewf_scim_user" "example" {
  user_name    = "alex@example.com"
  display_name = "Alex Example"
  active       = true
  emails       = [{ value = "alex@example.com", primary = true, type = "work" }]
}
