// Package apispec holds the published Workforce API contract used by the provider.
package apispec

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

const URL = "https://docs.merge.dev/openapi/workforce-api.json"

// Snapshot is the unmodified specification retrieved on 2026-11-05.
//
//go:embed workforce-api.json
var Snapshot []byte

type Schema struct {
	Ref         string             `json:"$ref"`
	Type        any                `json:"type"`
	Properties  map[string]*Schema `json:"properties"`
	Required    []string           `json:"required"`
	Items       *Schema            `json:"items"`
	OneOf       []*Schema          `json:"oneOf"`
	Enum        []string           `json:"enum"`
	Default     any                `json:"default"`
	MinLength   *int               `json:"minLength"`
	MaxLength   *int               `json:"maxLength"`
	Minimum     *float64           `json:"minimum"`
	Maximum     *float64           `json:"maximum"`
	Pattern     string             `json:"pattern"`
	Description string             `json:"description"`
}
type Parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"`
	Required bool    `json:"required"`
	Schema   *Schema `json:"schema"`
}
type Content struct {
	Schema *Schema `json:"schema"`
}
type Body struct {
	Content map[string]Content `json:"content"`
}
type Operation struct {
	OperationID string          `json:"operationId"`
	Description string          `json:"description"`
	Parameters  []Parameter     `json:"parameters"`
	RequestBody Body            `json:"requestBody"`
	Responses   map[string]Body `json:"responses"`
}
type Document struct {
	Paths      map[string]map[string]Operation `json:"paths"`
	Components struct {
		Schemas map[string]*Schema `json:"schemas"`
	} `json:"components"`
}

func Parse(data []byte) (*Document, error) {
	var d Document
	err := json.Unmarshal(data, &d)
	if err == nil && len(d.Paths) == 0 {
		err = fmt.Errorf("specification contains no paths")
	}
	return &d, err
}
func MustLoad() *Document {
	d, err := Parse(Snapshot)
	if err != nil {
		panic(err)
	}
	return d
}
func (d *Document) Resolve(s *Schema) *Schema {
	if s == nil {
		return &Schema{Type: "string"}
	}
	if s.Ref != "" {
		return d.Resolve(d.Components.Schemas[s.Ref[len("#/components/schemas/"):]])
	}
	if len(s.OneOf) > 0 {
		for _, v := range s.OneOf {
			if v.Kind() != "null" {
				return d.Resolve(v)
			}
		}
	}
	return s
}
func (s *Schema) Kind() string {
	switch t := s.Type.(type) {
	case string:
		return t
	case []any:
		for _, v := range t {
			if v != "null" {
				return v.(string)
			}
		}
	}
	return "string"
}
func (d *Document) Input(p, m string) *Schema {
	return d.Resolve(d.Paths[p][m].RequestBody.Content["application/json"].Schema)
}
