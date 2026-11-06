package main

import (
	"strings"
	"testing"

	"github.com/bogware/terraform-provider-mergewf/internal/apispec"
)

func TestCoverageAndNewOperations(t *testing.T) {
	d := apispec.MustLoad()
	if _, gaps := report(d); len(gaps) != 0 {
		t.Fatal(gaps)
	}
	d.Paths["/api/v1/new-feature"] = map[string]apispec.Operation{"get": {}}
	if _, gaps := report(d); len(gaps) != 1 || !strings.Contains(gaps[0], "Uncovered") {
		t.Fatal(gaps)
	}
}
func TestNewBodyFieldDetected(t *testing.T) {
	d := apispec.MustLoad()
	d.Components.Schemas["GroupNameRequest"].Properties["new_field"] = &apispec.Schema{Type: "string"}
	_, gaps := report(d)
	if len(gaps) != 2 {
		t.Fatalf("expected create and update gaps: %v", gaps)
	}
}
