// Generate examples from the registered provider schema.
package main

import (
	"context"
	"fmt"
	"github.com/bogware/terraform-provider-mergewf/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func write(name, content string) {
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		log.Fatal(err)
	}
}
func main() {
	write("examples/provider/provider.tf", `terraform {
  required_providers {
    mergewf = {
      source = "bogware/mergewf"
    }
  }
}

# Set MERGEWF_API_KEY and, for SCIM, MERGEWF_SCIM_TOKEN in the environment.
provider "mergewf" {}
`)
	samples := map[string]string{
		"group":                           `name = "Engineering"`,
		"group_member":                    "group_id = \"11111111-1111-4111-8111-111111111111\"\nuser_id = \"22222222-2222-4222-8222-222222222222\"",
		"tool_access_block":               "connector = \"github\"\ntool_class = \"DESTRUCTIVE\"\nreason = \"Prevent destructive GitHub operations\"\nlifecycle { create_before_destroy = true }",
		"person":                          "person_id = \"22222222-2222-4222-8222-222222222222\"\nname = \"Alex Example\"\nrole = \"MEMBER\"",
		"group_tool_access_policy":        "group_id = \"11111111-1111-4111-8111-111111111111\"\nmode = \"CUSTOM\"\nrules = [{action = \"ALLOW\", connector = \"github\", tool_class = \"READ_ONLY\"}]",
		"user_tool_access_policy":         "user_id = \"22222222-2222-4222-8222-222222222222\"\nmode = \"CUSTOM\"\nrules = [{action = \"ALLOW\", connector = \"github\", tool_class = \"READ_ONLY\"}]",
		"organization_tool_access_policy": "mode = \"NONE\"\nrules = []",
		"scim_group":                      "display_name = \"Engineering\"\nexternal_id = \"engineering\"\nmembers = []",
		"scim_user":                       "user_name = \"alex@example.com\"\ndisplay_name = \"Alex Example\"\nactive = true\nemails = [{value = \"alex@example.com\", primary = true, type = \"work\"}]",
		"skill_enablement":                "slug = \"merge-example-skill\"\nis_enabled = true\nscope = \"ORG\"\ngroup_ids = []\nperson_ids = []",
		"skill_person_grant":              "slug = \"example-skill\"\nperson_id = \"22222222-2222-4222-8222-222222222222\"",
		"skill_configuration":             "slug = \"example-skill\"\nname = \"Example skill\"\ndescription = \"An existing uploaded skill managed by Terraform\"\nis_active = true\naccess_scope = \"ORG\"\ngroup_ids = []",
	}
	imports := map[string]string{"group_member": "GROUP_ID/USER_ID", "person": "PERSON_ID", "group_tool_access_policy": "GROUP_ID", "user_tool_access_policy": "USER_ID", "organization_tool_access_policy": "organization", "skill_enablement": "SKILL_SLUG", "skill_person_grant": "SKILL_SLUG/PERSON_ID", "skill_configuration": "SKILL_SLUG"}
	server := providerserver.NewProtocol6(provider.New("docs")())()
	s, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range s.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			log.Fatal(d.Summary, ": ", d.Detail)
		}
	}
	for name := range s.ResourceSchemas {
		short := strings.TrimPrefix(name, "mergewf_")
		body, ok := samples[short]
		if !ok {
			log.Fatalf("missing resource example: %s", name)
		}
		directory := "examples/resources/" + name
		write(directory+"/resource.tf", fmt.Sprintf("resource %q \"example\" {\n%s\n}\n", name, body))
		id := imports[short]
		if id == "" {
			id = "RESOURCE_ID"
		}
		write(directory+"/import.sh", fmt.Sprintf("terraform import %s.example %s\n", name, id))
	}
	for name, schema := range s.DataSourceSchemas {
		body := ""
		for _, a := range schema.Block.Attributes {
			if a.Required {
				value := "11111111-1111-4111-8111-111111111111"
				if a.Name == "slug" {
					value = "example-skill"
				}
				body += fmt.Sprintf("%s = %q\n", a.Name, value)
			}
		}
		write("examples/data-sources/"+name+"/data-source.tf", fmt.Sprintf("data %q \"example\" {\n%s}\n\noutput \"result\" {\n  value = jsondecode(data.%s.example.result_json)\n}\n", name, body, name))
	}
}
