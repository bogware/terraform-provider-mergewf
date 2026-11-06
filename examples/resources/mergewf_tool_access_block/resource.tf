resource "mergewf_tool_access_block" "example" {
  connector  = "github"
  tool_class = "DESTRUCTIVE"
  reason     = "Prevent destructive GitHub operations"
  lifecycle { create_before_destroy = true }
}
