terraform {
  required_providers {
    mergewf = {
      source = "bogware/mergewf"
    }
  }
}

# Set MERGEWF_API_KEY and, for SCIM, MERGEWF_SCIM_TOKEN in the environment.
provider "mergewf" {}
