// Export protocol schemas for tfplugindocs without installing a development
// provider through a temporary filesystem mirror (unreliable on Windows).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bogware/terraform-provider-mergewf/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func attributes(list []*tfprotov6.SchemaAttribute) map[string]any {
	out := map[string]any{}
	for _, a := range list {
		v := map[string]any{"description": a.Description, "description_kind": "markdown", "required": a.Required, "optional": a.Optional, "computed": a.Computed, "sensitive": a.Sensitive}
		if a.NestedType != nil {
			v["nested_type"] = map[string]any{"nesting_mode": strings.ToLower(a.NestedType.Nesting.String()), "attributes": attributes(a.NestedType.Attributes)}
		} else {
			raw, err := a.Type.MarshalJSON()
			if err != nil {
				log.Fatal(err)
			}
			v["type"] = json.RawMessage(raw)
		}
		out[a.Name] = v
	}
	return out
}
func schema(s *tfprotov6.Schema) map[string]any {
	return map[string]any{"version": s.Version, "block": map[string]any{"description": s.Block.Description, "description_kind": "markdown", "attributes": attributes(s.Block.Attributes)}}
}
func main() {
	output := flag.String("output", ".cache/provider-schema.json", "Output file")
	flag.Parse()
	server := providerserver.NewProtocol6(provider.New("docs")())()
	resp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			log.Fatalf("%s: %s", d.Summary, d.Detail)
		}
	}
	resources := map[string]any{}
	for name, s := range resp.ResourceSchemas {
		resources[name] = schema(s)
	}
	dataSources := map[string]any{}
	for name, s := range resp.DataSourceSchemas {
		dataSources[name] = schema(s)
	}
	result := map[string]any{"format_version": "1.0", "provider_schemas": map[string]any{"mergewf": map[string]any{"provider": schema(resp.Provider), "resource_schemas": resources, "data_source_schemas": dataSources}}}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*output, data, 0644); err != nil {
		log.Fatal(err)
	}
}
