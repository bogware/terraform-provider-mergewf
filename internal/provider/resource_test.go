package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bogware/terraform-provider-mergewf/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestReadDistinguishesMissingFromForbidden(t *testing.T) {
	for _, status := range []int{403, 404, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctx := context.Background()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			c, _ := client.New(server.URL, "fixture", "", "test")
			r := &apiResource{spec: resources()[0], client: c}
			var schema resource.SchemaResponse
			r.Schema(ctx, resource.SchemaRequest{}, &schema)
			state := tfsdk.State{Schema: schema.Schema}
			attrs := map[string]attr.Type{}
			for k, a := range schema.Schema.Attributes {
				attrs[k] = a.GetType()
			}
			value := types.ObjectValueMust(attrs, map[string]attr.Value{"id": types.StringValue("group-1"), "name": types.StringValue("Existing"), "result_json": types.StringValue(`{"id":"group-1","name":"Existing"}`)})
			if d := state.Set(ctx, value); d.HasError() {
				t.Fatal(d)
			}
			response := resource.ReadResponse{State: state}
			r.Read(ctx, resource.ReadRequest{State: state}, &response)
			if status == 404 {
				if !response.State.Raw.IsNull() || response.Diagnostics.HasError() {
					t.Fatalf("404 did not remove missing object: %v", response.Diagnostics)
				}
			} else {
				if !response.Diagnostics.HasError() || !response.State.Raw.Equal(state.Raw) {
					t.Fatal("non-404 error discarded state")
				}
			}
		})
	}
}
