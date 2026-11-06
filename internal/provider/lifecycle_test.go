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
	commands map[string]int
	requests []string
}

func (f *resourceFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
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
	if f.spec.Name == "person" && r.Method == "POST" {
		command := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if f.object == nil || (command != "deactivate" && command != "reactivate") {
			w.WriteHeader(404)
			return
		}
		if f.commands == nil {
			f.commands = map[string]int{}
		}
		f.commands[command]++
		if command == "deactivate" {
			f.object["status"] = "DEACTIVATED"
		} else {
			f.object["status"] = "ACTIVE"
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
		if f.spec.Name == "person" {
			f.object["status"] = "ACTIVE"
		}
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

func personSpec(t *testing.T) resourceSpec {
	for _, spec := range resources() {
		if spec.Name == "person" {
			return spec
		}
	}
	t.Fatal("person resource missing")
	return resourceSpec{}
}

// TestFixturePersonActive covers deactivation, reactivation, drift, omission,
// and invited people for the person active attribute.
func TestFixturePersonActive(t *testing.T) {
	fixture := &resourceFixture{t: t, spec: personSpec(t), object: map[string]any{"id": "person-1", "name": "Person", "role": "MEMBER", "status": "ACTIVE"}}
	server := httptest.NewServer(fixture)
	defer server.Close()
	config := func(body string) string {
		return fmt.Sprintf("provider \"mergewf\" {\napi_key = \"fixture-key\"\nbase_url = %q\n}\nresource \"mergewf_person\" \"test\" {\nperson_id = \"person-1\"\n%s\n}\n", server.URL, body)
	}
	expect := func(deactivations, reactivations int, status string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.commands["deactivate"] != deactivations || fixture.commands["reactivate"] != reactivations {
				return fmt.Errorf("commands=%v, want deactivate=%d reactivate=%d", fixture.commands, deactivations, reactivations)
			}
			if fixture.object["status"] != status {
				return fmt.Errorf("status=%v, want %s", fixture.object["status"], status)
			}
			return nil
		}
	}
	// before checks that the most recent first request preceded the most recent second request.
	before := func(first, second string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			a, b := -1, -1
			for i, request := range fixture.requests {
				if request == first {
					a = i
				}
				if request == second {
					b = i
				}
			}
			if a < 0 || b < 0 || a > b {
				return fmt.Errorf("want %q before %q in %v", first, second, fixture.requests)
			}
			return nil
		}
	}
	patch, deactivate, reactivate := "PATCH /api/v1/persons/person-1", "POST /api/v1/persons/person-1/deactivate", "POST /api/v1/persons/person-1/reactivate"
	setStatus := func(status string) func() {
		return func() { fixture.mu.Lock(); defer fixture.mu.Unlock(); fixture.object["status"] = status }
	}
	address := "mergewf_person.test"
	resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: factories, Steps: []resource.TestStep{
		{Config: config(`active = false`), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr(address, "active", "false"), expect(1, 0, "DEACTIVATED"), before(patch, deactivate))},
		{Config: config(`active = false`), PlanOnly: true},
		{ResourceName: address, ImportState: true, ImportStateId: "person-1", ImportStateVerify: true},
		// Renaming and reactivating together reactivates before editing.
		{Config: config("active = true\nname = \"Renamed\""), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr(address, "active", "true"), resource.TestCheckResourceAttr(address, "name", "Renamed"), expect(1, 1, "ACTIVE"), before(reactivate, patch))},
		// Out-of-band deactivation is drift that apply corrects.
		{PreConfig: setStatus("DEACTIVATED"), Config: config("active = true\nname = \"Renamed\""), Check: expect(1, 2, "ACTIVE")},
		// Omitting active leaves the status unmanaged.
		{PreConfig: setStatus("DEACTIVATED"), Config: config(`name = "Unmanaged"`), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr(address, "active", "false"), expect(1, 2, "DEACTIVATED"))},
		// Invited people are active; no reactivation is attempted.
		{PreConfig: setStatus("INVITED"), Config: config(`active = true`), Check: resource.ComposeTestCheckFunc(resource.TestCheckResourceAttr(address, "active", "true"), expect(1, 2, "INVITED"))},
		{Config: config(`active = true`), PlanOnly: true},
	}, CheckDestroy: func(*terraform.State) error {
		fixture.mu.Lock()
		defer fixture.mu.Unlock()
		if fixture.object == nil || fixture.object["status"] != "INVITED" {
			return fmt.Errorf("destroy changed the person: %v", fixture.object)
		}
		return nil
	}})
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
