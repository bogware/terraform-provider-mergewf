package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bogware/terraform-provider-mergewf/internal/apispec"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func nullTerraform(ctx context.Context, t attr.Type) tftypes.Value {
	return tftypes.NewValue(t.TerraformType(ctx), nil)
}

func nodeType(node *apispec.Schema) attr.Type {
	n := contract.Resolve(node)
	switch n.Kind() {
	case "boolean":
		return types.BoolType
	case "integer":
		return types.Int64Type
	case "array":
		return types.ListType{ElemType: nodeType(n.Items)}
	case "object":
		attrs := map[string]attr.Type{}
		for k, v := range n.Properties {
			attrs[snake(k)] = nodeType(v)
		}
		return types.ObjectType{AttrTypes: attrs}
	default:
		return types.StringType
	}
}
func resourceAttribute(node *apispec.Schema, required, computed, replace bool, description string) schema.Attribute {
	n := contract.Resolve(node)
	optional := !required
	if n.Description != "" && !strings.Contains(n.Description, "plans/") {
		description += " " + n.Description
	}
	if len(n.Enum) > 0 {
		description += " Allowed values: `" + strings.Join(n.Enum, "`, `") + "`."
	}
	if n.MinLength != nil {
		description += fmt.Sprintf(" Minimum length: %d.", *n.MinLength)
	}
	if n.MaxLength != nil {
		description += fmt.Sprintf(" Maximum length: %d.", *n.MaxLength)
	}
	switch n.Kind() {
	case "boolean":
		a := schema.BoolAttribute{Required: required, Optional: optional, Computed: computed, MarkdownDescription: description}
		if computed {
			a.PlanModifiers = append(a.PlanModifiers, boolplanmodifier.UseStateForUnknown())
		}
		if replace {
			a.PlanModifiers = append(a.PlanModifiers, boolplanmodifier.RequiresReplace())
		}
		return a
	case "integer":
		a := schema.Int64Attribute{Required: required, Optional: optional, Computed: computed, MarkdownDescription: description}
		if computed {
			a.PlanModifiers = append(a.PlanModifiers, int64planmodifier.UseStateForUnknown())
		}
		if replace {
			a.PlanModifiers = append(a.PlanModifiers, int64planmodifier.RequiresReplace())
		}
		return a
	case "object":
		attrs := map[string]schema.Attribute{}
		for k, v := range n.Properties {
			attrs[snake(k)] = resourceAttribute(v, contains(n.Required, k), false, false, k+".")
		}
		a := schema.SingleNestedAttribute{Required: required, Optional: optional, Computed: computed, Attributes: attrs, MarkdownDescription: description}
		if computed {
			a.PlanModifiers = append(a.PlanModifiers, objectplanmodifier.UseStateForUnknown())
		}
		if replace {
			a.PlanModifiers = append(a.PlanModifiers, objectplanmodifier.RequiresReplace())
		}
		return a
	case "array":
		item := contract.Resolve(n.Items)
		if item.Kind() == "object" {
			attrs := map[string]schema.Attribute{}
			for k, v := range item.Properties {
				attrs[snake(k)] = resourceAttribute(v, contains(item.Required, k), false, false, k+".")
			}
			a := schema.ListNestedAttribute{Required: required, Optional: optional, Computed: computed, NestedObject: schema.NestedAttributeObject{Attributes: attrs}, MarkdownDescription: description}
			if computed {
				a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.UseStateForUnknown())
			}
			if replace {
				a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.RequiresReplace())
			}
			return a
		}
		a := schema.ListAttribute{Required: required, Optional: optional, Computed: computed, ElementType: nodeType(item), MarkdownDescription: description}
		if computed {
			a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.UseStateForUnknown())
		}
		if replace {
			a.PlanModifiers = append(a.PlanModifiers, listplanmodifier.RequiresReplace())
		}
		return a
	default:
		a := schema.StringAttribute{Required: required, Optional: optional, Computed: computed, MarkdownDescription: description}
		if computed {
			a.PlanModifiers = append(a.PlanModifiers, stringplanmodifier.UseStateForUnknown())
		}
		if replace {
			a.PlanModifiers = append(a.PlanModifiers, stringplanmodifier.RequiresReplace())
		}
		return a
	}
}

func native(node *apispec.Schema, value attr.Value) any {
	if value == nil || value.IsNull() || value.IsUnknown() {
		return nil
	}
	n := contract.Resolve(node)
	switch v := value.(type) {
	case types.String:
		return v.ValueString()
	case types.Bool:
		return v.ValueBool()
	case types.Int64:
		return v.ValueInt64()
	case types.List:
		result := []any{}
		for _, item := range v.Elements() {
			result = append(result, native(n.Items, item))
		}
		return result
	case types.Object:
		result := map[string]any{}
		for k, child := range n.Properties {
			item := v.Attributes()[snake(k)]
			if item != nil && !item.IsNull() && !item.IsUnknown() {
				result[k] = native(child, item)
			}
		}
		return result
	}
	return nil
}

// terraformValue projects API data onto the request shape. Optional nested fields
// omitted by the user remain null even when the API supplies extra defaults.
func terraformValue(ctx context.Context, node *apispec.Schema, data any, prior attr.Value) (attr.Value, error) {
	n := contract.Resolve(node)
	typ := nodeType(n)
	if data == nil {
		v, err := typ.ValueFromTerraform(ctx, nullTerraform(ctx, typ))
		return v, err
	}
	switch n.Kind() {
	case "string":
		v, ok := data.(string)
		if !ok {
			return nil, fmt.Errorf("expected string response")
		}
		return types.StringValue(v), nil
	case "boolean":
		v, ok := data.(bool)
		if !ok {
			return nil, fmt.Errorf("expected boolean response")
		}
		return types.BoolValue(v), nil
	case "integer":
		v, ok := data.(float64)
		if !ok {
			return nil, fmt.Errorf("expected integer response")
		}
		return types.Int64Value(int64(v)), nil
	case "array":
		items, ok := data.([]any)
		if !ok {
			return nil, fmt.Errorf("expected array response")
		}
		result := []attr.Value{}
		var old []attr.Value
		if p, ok := prior.(types.List); ok {
			old = p.Elements()
		}
		for i, item := range items {
			var p attr.Value
			if i < len(old) {
				p = old[i]
			}
			v, err := terraformValue(ctx, n.Items, item, p)
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		}
		v, d := types.ListValue(nodeType(n.Items), result)
		if d.HasError() {
			return nil, fmt.Errorf("invalid list response: %s", d)
		}
		return v, nil
	case "object":
		obj, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object response")
		}
		result := map[string]attr.Value{}
		var old map[string]attr.Value
		if p, ok := prior.(types.Object); ok && !p.IsNull() && !p.IsUnknown() {
			old = p.Attributes()
		}
		for k, child := range n.Properties {
			name := snake(k)
			item := obj[k]
			p := old[name]
			if p != nil && p.IsNull() && !contains(n.Required, k) {
				item = nil
			}
			v, err := terraformValue(ctx, child, item, p)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			result[name] = v
		}
		v, d := types.ObjectValue(typ.(types.ObjectType).AttrTypes, result)
		if d.HasError() {
			return nil, fmt.Errorf("invalid object response: %s", d)
		}
		return v, nil
	}
	return nil, fmt.Errorf("unsupported schema type %s", n.Kind())
}

func validateNode(node *apispec.Schema, v attr.Value, p path.Path, d *diag.Diagnostics) {
	if v == nil || v.IsNull() || v.IsUnknown() {
		return
	}
	n := contract.Resolve(node)
	fail := func(message string) { d.AddAttributeError(p, "Invalid configuration", message) }
	switch value := v.(type) {
	case types.String:
		s := value.ValueString()
		if len(n.Enum) > 0 && !contains(n.Enum, s) {
			fail(fmt.Sprintf("Must be one of %v.", n.Enum))
		}
		length := utf8.RuneCountInString(s)
		if n.MinLength != nil && length < *n.MinLength {
			fail(fmt.Sprintf("Must contain at least %d characters.", *n.MinLength))
		}
		if n.MaxLength != nil && length > *n.MaxLength {
			fail(fmt.Sprintf("Must contain at most %d characters.", *n.MaxLength))
		}
		if n.Pattern != "" {
			if re, err := regexp.Compile(n.Pattern); err == nil && !re.MatchString(s) {
				fail("Value does not match the API's required format.")
			}
		}
	case types.Int64:
		x := float64(value.ValueInt64())
		if n.Minimum != nil && x < *n.Minimum {
			fail(fmt.Sprintf("Must be at least %g.", *n.Minimum))
		}
		if n.Maximum != nil && x > *n.Maximum {
			fail(fmt.Sprintf("Must be at most %g.", *n.Maximum))
		}
	case types.List:
		for i, item := range value.Elements() {
			validateNode(n.Items, item, p.AtListIndex(i), d)
		}
	case types.Object:
		for k, child := range n.Properties {
			validateNode(child, value.Attributes()[snake(k)], p.AtName(snake(k)), d)
		}
	}
}
