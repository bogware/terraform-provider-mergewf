resource "mergewf_skill_configuration" "example" {
  slug         = "example-skill"
  name         = "Example skill"
  description  = "An existing uploaded skill managed by Terraform"
  is_active    = true
  access_scope = "ORG"
  group_ids    = []
}
