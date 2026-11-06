resource "mergewf_skill_enablement" "example" {
  slug       = "merge-example-skill"
  is_enabled = true
  scope      = "ORG"
  group_ids  = []
  person_ids = []
}
