package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var factories = map[string]func() (tfprotov6.ProviderServer, error){"mergewf": providerserver.NewProtocol6WithError(New("test")())}

func TestProtocolSchemas(t *testing.T) {
	server, err := factories["mergewf"]()
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range response.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Errorf("%s: %s", d.Summary, d.Detail)
		}
	}
	if len(response.ResourceSchemas) != 12 {
		t.Errorf("resources=%d", len(response.ResourceSchemas))
	}
	if len(response.DataSourceSchemas) != 26 {
		t.Errorf("data sources=%d", len(response.DataSourceSchemas))
	}
	if response.Provider.Block.Attributes[0].Name == "endpoint" {
		t.Fatal("scaffolding schema remains")
	}
}
