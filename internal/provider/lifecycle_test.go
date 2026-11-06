package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// These tests run the real Terraform CLI against an in-process HTTP fixture.
// No live credentials, TF_ACC switch, or Merge account is used.
func TestFixtureResourceLifecycles(t *testing.T) {
	samples := map[string]string{
		"group":                           `name = "Engineering one"`,
		"group_member":                    "group_id = \"group-1\"\nuser_id = \"user-1\"",
		"tool_access_block":               "connector = \"github\"\nreason = \"reason one\"",
		"person":                          "person_id = \"person-1\"\nname = \"Person one\"\nrole = \"MEMBER\"",
		"group_tool_access_policy":        "group_id = \"group-1\"\nmode = \"CUSTOM\"\nrules = [{action = \"ALLOW\", connector = \"github\"}]",
		"user_tool_access_policy":         "user_id = \"user-1\"\nmode = \"CUSTOM\"\nrules = [{action = \"ALLOW\", connector = \"github\"}]",
		"organization_tool_access_policy": "mode = \"CUSTOM\"\nrules = [{action = \"ALLOW\", connector = \"github\"}]",
		"scim_group":                      "display_name = \"Engineering one\"\nexternal_id = \"external-group\"\nmembers = [{value = \"user-1\"}]",
		"scim_user":                       "user_name = \"employee@example.test\"\ndisplay_name = \"Employee one\"\nactive = true\nemails = [{value = \"employee@example.test\", primary = true}]\nname = {given_name = \"Employee\", family_name = \"One\"}",
		"skill_enablement":                "slug = \"merge-example\"\nis_enabled = true\nscope = \"GROUP\"\ngroup_ids = [\"group-1\"]\nperson_ids = []",
		"skill_person_grant":              "slug = \"example\"\nperson_id = \"person-1\"",
		"skill_configuration":             "slug = \"example\"\nname = \"Skill one\"\ndescription = \"Example skill\"\nis_active = true\naccess_scope = \"GROUP\"\ngroup_ids = [\"group-1\"]\nreferenced_connectors = [\"github\"]",
	}
	for _, spec := range resources() {
		t.Run(spec.Name, func(t *testing.T) {
			fixture := &resourceFixture{t: t, spec: spec}
			server := httptest.NewServer(fixture)
			defer server.Close()
			config := func(body string) string {
				return fmt.Sprintf("provider \"mergewf\" {\napi_key = \"fixture-key\"\nscim_token = \"fixture-scim\"\nbase_url = %q\n}\nresource \"mergewf_%s\" \"test\" {\n%s\n}\n", server.URL, spec.Name, body)
			}
			original := samples[spec.Name]
			changed := strings.ReplaceAll(original, " one", " two")
			if spec.Kind == "policy" {
				changed = strings.ReplaceAll(original, "ALLOW", "DENY")
			}
			if spec.Kind == "enablement" {
				changed = strings.ReplaceAll(original, "group-1", "group-2")
			}
			address := "mergewf_" + spec.Name + ".test"
			steps := []resource.TestStep{{Config: config(original), Check: resource.TestCheckResourceAttrSet(address, "id")}, {ResourceName: address, ImportState: true, ImportStateVerify: true}, {Config: config(changed)}}
			// A second plan detects responses that accidentally rewrite configured values.
			steps = append(steps, resource.TestStep{Config: config(changed), PlanOnly: true})
			if spec.Kind == "enablement" {
				disabled := strings.ReplaceAll(changed, "is_enabled = true", "is_enabled = false")
				steps = append(steps, resource.TestStep{Config: config(disabled), Check: resource.TestCheckResourceAttr(address, "is_enabled", "false")}, resource.TestStep{Config: config(changed), Check: resource.TestCheckResourceAttr(address, "is_enabled", "true")})
			}
			if spec.Name == "scim_group" {
				// Omission must retain members during PUT; an explicit [] clears them.
				omitted := strings.ReplaceAll(changed, "members = [{value = \"user-1\"}]", "")
				omitted = strings.ReplaceAll(omitted, "two", "three")
				steps = append(steps, resource.TestStep{Config: config(omitted), Check: resource.TestCheckResourceAttr(address, "members.#", "1")}, resource.TestStep{Config: config(omitted + "\nmembers = []"), Check: resource.TestCheckResourceAttr(address, "members.#", "0")})
			}
			if spec.Kind == "policy" {
				cleared := strings.ReplaceAll(changed, `rules = [{action = "DENY", connector = "github"}]`, "rules = []")
				steps = append(steps, resource.TestStep{Config: config(cleared), Check: resource.TestCheckResourceAttr(address, "rules.#", "0")})
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: steps, CheckDestroy: func(*terraform.State) error {
				fixture.mu.Lock()
				defer fixture.mu.Unlock()
				if spec.DeleteMethod != "" && fixture.object != nil {
					return fmt.Errorf("remote resource still exists")
				}
				if spec.Kind == "enablement" && lookup(fixture.object, "reach.scope") != nil {
					return fmt.Errorf("skill not disabled")
				}
				if (spec.Kind == "adopt" || spec.Name == "organization_tool_access_policy") && fixture.object == nil {
					return fmt.Errorf("adopted object was deleted")
				}
				return nil
			}})
		})
	}
}

func TestFixtureInvalidConfiguration(t *testing.T) {
	for _, test := range []struct{ name, body, want string }{
		{"group", `name = ""`, "Must contain at least"},
		{"organization_tool_access_policy", `mode = "INVALID"`, "Must be one of"},
		{"tool_access_block", `reason = "No selector"`, "Missing block selector"},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := fmt.Sprintf("provider \"mergewf\" {}\nresource \"mergewf_%s\" \"test\" {\n%s\n}", test.name, test.body)
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile(test.want)}}})
		})
	}
}

type resourceFixture struct {
	t        *testing.T
	mu       sync.Mutex
	spec     resourceSpec
	object   map[string]any
	sequence int
}

func (f *resourceFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	want := "Bearer fixture-key"
	if strings.HasPrefix(r.URL.Path, "/scim/") {
		want = "Bearer fixture-scim"
	}
	if r.Header.Get("Authorization") != want {
		f.t.Errorf("wrong credential on %s", r.URL.Path)
		w.WriteHeader(401)
		return
	}
	if r.Method == "GET" {
		if f.spec.ListRoute != "" {
			items := []any{}
			if f.object != nil {
				items = append(items, f.object)
			}
			if f.spec.Kind == "member" {
				json.NewEncoder(w).Encode(map[string]any{"results": items, "next_cursor": nil})
			} else {
				json.NewEncoder(w).Encode(items)
			}
			return
		}
		if f.object == nil {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(f.object)
		return
	}
	if r.Method == "DELETE" {
		f.object = nil
		w.WriteHeader(204)
		return
	}
	var body map[string]any
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	if f.object == nil {
		f.sequence++
		f.object = map[string]any{"id": fmt.Sprintf("fixture-%d", f.sequence)}
	}
	switch f.spec.Kind {
	case "member":
		ids, _ := body["user_ids"].([]any)
		if len(ids) != 1 {
			f.t.Errorf("wrong membership payload: %v", body)
		}
		f.object = map[string]any{"user_id": ids[0], "name": "Member", "type": "PERSON", "source": "API", "email": "member@example.test"}
	case "grant":
		f.object = map[string]any{"slug": "example", "reach": map[string]any{"pinned_person_ids": []any{"person-1"}}}
		w.WriteHeader(204)
		return
	case "enablement":
		scope := body["scope"]
		if body["is_enabled"] == false {
			scope = nil
		}
		f.object = map[string]any{"slug": "merge-example", "is_global": true, "reach": map[string]any{"scope": scope, "group_ids": body["group_ids"], "person_ids": body["person_ids"]}}
	default:
		for k, v := range body {
			f.object[k] = v
		}
		if f.spec.Kind == "policy" {
			if _, ok := f.object["rules"]; !ok {
				f.object["rules"] = []any{}
			}
		}
		if f.spec.Name == "skill_configuration" {
			f.object["reach"] = map[string]any{"scope": f.object["access_scope"], "group_ids": f.object["group_ids"]}
			delete(f.object, "access_scope")
			delete(f.object, "group_ids")
		}
	}
	json.NewEncoder(w).Encode(f.object)
}

func TestFixtureDriftAndRemoteDeletion(t *testing.T) {
	fixture := &resourceFixture{t: t, spec: resources()[0]}
	server := httptest.NewServer(fixture)
	defer server.Close()
	config := fmt.Sprintf("provider \"mergewf\" {\napi_key = \"fixture-key\"\nbase_url = %q\n}\nresource \"mergewf_group\" \"test\" { name = \"Managed\" }", server.URL)
	mutate := func(object map[string]any) func() {
		return func() { fixture.mu.Lock(); defer fixture.mu.Unlock(); fixture.object = object }
	}
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config},
		{PreConfig: mutate(map[string]any{"id": "fixture-1", "name": "Drifted"}), Config: config, Check: resource.TestCheckResourceAttr("mergewf_group.test", "name", "Managed")},
		{PreConfig: mutate(nil), Config: config, Check: resource.TestCheckResourceAttr("mergewf_group.test", "id", "fixture-2")},
	}})
}

func TestFixtureDataSources(t *testing.T) {
	for _, spec := range dataSources() {
		t.Run(spec.Name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" {
					t.Errorf("unexpected %s", r.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				if !spec.List {
					fmt.Fprint(w, `{"id":"value","name":"Example"}`)
					return
				}
				if strings.HasPrefix(spec.Route, "/scim/") {
					fmt.Fprint(w, `{"Resources":[{"id":"value"}],"totalResults":1}`)
				} else if contract.Resolve(contract.Paths[spec.Route]["get"].Responses["200"].Content["application/json"].Schema).Kind() == "array" {
					fmt.Fprint(w, `[{"id":"value"}]`)
				} else {
					fmt.Fprint(w, `{"results":[{"id":"value"}],"next_cursor":null}`)
				}
			}))
			defer server.Close()
			params := ""
			for _, p := range spec.Parameters {
				if p.Required {
					params += fmt.Sprintf("%s = %q\n", snake(p.Name), "fixture-id")
				}
			}
			config := fmt.Sprintf("provider \"mergewf\" {\napi_key = \"fixture-key\"\nscim_token = \"fixture-scim\"\nbase_url = %q\n}\ndata \"mergewf_%s\" \"test\" {\n%s}\n", server.URL, spec.Name, params)
			want := `{"id":"value","name":"Example"}`
			if spec.List {
				want = `[{"id":"value"}]`
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{{Config: config, Check: resource.TestCheckResourceAttr("data.mergewf_"+spec.Name+".test", "result_json", want)}}})
			if calls == 0 {
				t.Fatal("no API request")
			}
		})
	}
}
