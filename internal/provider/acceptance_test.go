package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/bogware/terraform-provider-mergewf/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Acceptance tests run against a live Workforce tenant and change it. They use
// resource.Test, which skips them unless TF_ACC is set, so `go test ./...`
// stays fixture-only. See docs/guides/acceptance-testing.md for setup.
//
// Required: TF_ACC=1, MERGEWF_API_KEY.
// Optional: MERGEWF_BASE_URL, MERGEWF_ACC_USER_ID (an existing user added to a
// throwaway group), MERGEWF_ACC_PERSON_ID (a disposable, manually created
// person that is deactivated and then reactivated), MERGEWF_SCIM_TOKEN.

func accPreCheck(t *testing.T, vars ...string) {
	t.Helper()
	for _, v := range append([]string{"MERGEWF_API_KEY"}, vars...) {
		if os.Getenv(v) == "" {
			t.Skipf("%s is not set", v)
		}
	}
}

func accClient(t *testing.T) *client.Client {
	t.Helper()
	base := os.Getenv("MERGEWF_BASE_URL")
	if base == "" {
		base = client.DefaultBaseURL
	}
	c, err := client.New(base, os.Getenv("MERGEWF_API_KEY"), os.Getenv("MERGEWF_SCIM_TOKEN"), "acceptance")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// accGone verifies that a destroyed object returns HTTP 404. Refresh removes
// state only on 404, so any other status would leave orphaned state.
func accGone(t *testing.T, resourceType, route string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := accClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			path := fmt.Sprintf(route, rs.Primary.ID)
			_, err := c.Do(context.Background(), "GET", path, nil, nil)
			if err == nil {
				return fmt.Errorf("%s still exists after destroy", path)
			}
			if !client.IsNotFound(err) {
				return fmt.Errorf("GET %s after destroy returned %v; refresh only treats 404 as deleted", path, err)
			}
		}
		return nil
	}
}

func TestAccGroupMembershipAndPolicy(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(8)
	userID := os.Getenv("MERGEWF_ACC_USER_ID")
	config := func(groupName, action string) string {
		member := ""
		if userID != "" {
			member = fmt.Sprintf(`
resource "mergewf_group_member" "test" {
  group_id = mergewf_group.test.id
  user_id  = %q
}
`, userID)
		}
		return fmt.Sprintf(`
provider "mergewf" {}

resource "mergewf_group" "test" {
  name = %q
}

resource "mergewf_group_tool_access_policy" "test" {
  group_id = mergewf_group.test.id
  mode     = "CUSTOM"
  rules = [{
    action     = %q
    connector  = "github"
    tool_class = "READ_ONLY"
  }]
}
%s`, groupName, action, member)
	}
	steps := []resource.TestStep{
		{Config: config(name, "ALLOW"), Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("mergewf_group.test", "name", name),
			resource.TestCheckResourceAttrSet("mergewf_group.test", "id"),
			resource.TestCheckResourceAttr("mergewf_group_tool_access_policy.test", "rules.#", "1"),
		)},
		// A second plan catches responses that rewrite configured values.
		{Config: config(name, "ALLOW"), PlanOnly: true},
		{ResourceName: "mergewf_group.test", ImportState: true, ImportStateVerify: true},
		{ResourceName: "mergewf_group_tool_access_policy.test", ImportState: true, ImportStateVerify: true},
	}
	if userID != "" {
		steps = append(steps, resource.TestStep{ResourceName: "mergewf_group_member.test", ImportState: true, ImportStateVerify: true})
	}
	steps = append(steps,
		resource.TestStep{Config: config(name+"-renamed", "DENY"), Check: resource.ComposeTestCheckFunc(
			resource.TestCheckResourceAttr("mergewf_group.test", "name", name+"-renamed"),
			resource.TestCheckResourceAttr("mergewf_group_tool_access_policy.test", "rules.0.action", "DENY"),
		)},
		resource.TestStep{Config: config(name+"-renamed", "DENY"), PlanOnly: true},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps:                    steps,
		CheckDestroy:             accGone(t, "mergewf_group", "/api/v1/groups/%s"),
	})
}

func TestAccPersonActive(t *testing.T) {
	personID := os.Getenv("MERGEWF_ACC_PERSON_ID")
	config := func(active bool) string {
		return fmt.Sprintf(`
provider "mergewf" {}

resource "mergewf_person" "test" {
  person_id = %q
  active    = %t
}
`, personID, active)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { accPreCheck(t, "MERGEWF_ACC_PERSON_ID") },
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{Config: config(true), Check: resource.TestCheckResourceAttr("mergewf_person.test", "active", "true")},
			{Config: config(false), Check: resource.ComposeTestCheckFunc(
				resource.TestCheckResourceAttr("mergewf_person.test", "active", "false"),
				resource.TestCheckResourceAttrWith("mergewf_person.test", "result_json", func(v string) error {
					if !strings.Contains(v, `"status":"DEACTIVATED"`) {
						return fmt.Errorf("person not deactivated: %s", v)
					}
					return nil
				}),
			)},
			{Config: config(false), PlanOnly: true},
			{ResourceName: "mergewf_person.test", ImportState: true, ImportStateId: personID, ImportStateVerify: true},
			// Always finish reactivated: destroy leaves status unchanged.
			{Config: config(true), Check: resource.TestCheckResourceAttr("mergewf_person.test", "active", "true")},
			{Config: config(true), PlanOnly: true},
		},
		CheckDestroy: func(*terraform.State) error {
			data, err := accClient(t).Do(context.Background(), "GET", "/api/v1/persons/"+personID, nil, nil)
			if err != nil {
				return err
			}
			if obj, _ := data.(map[string]any); obj["status"] == "DEACTIVATED" {
				return fmt.Errorf("person %s was left DEACTIVATED; reactivate it manually", personID)
			}
			return nil
		},
	})
}

// TestAccReadOnlyDataSources exercises pagination and response decoding
// without changing the tenant.
func TestAccReadOnlyDataSources(t *testing.T) {
	names := []string{"persons", "groups", "invites", "tool_access_blocks", "organization_tool_access_policy", "tool_access_requests", "skills", "skill_publishable_groups"}
	config := "provider \"mergewf\" {}\n"
	checks := []resource.TestCheckFunc{}
	for _, n := range names {
		config += fmt.Sprintf("data \"mergewf_%s\" \"test\" {}\n", n)
		checks = append(checks, resource.TestCheckResourceAttrSet("data.mergewf_"+n+".test", "result_json"))
	}
	if os.Getenv("MERGEWF_SCIM_TOKEN") != "" {
		for _, n := range []string{"scim_service_provider_config", "scim_users", "scim_groups"} {
			config += fmt.Sprintf("data \"mergewf_%s\" \"test\" {}\n", n)
			checks = append(checks, resource.TestCheckResourceAttrSet("data.mergewf_"+n+".test", "result_json"))
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { accPreCheck(t) },
		ProtoV6ProviderFactories: factories,
		Steps:                    []resource.TestStep{{Config: config, Check: resource.ComposeTestCheckFunc(checks...)}},
	})
}
