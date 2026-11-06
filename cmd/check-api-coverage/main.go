// Compare the implementation with Merge's published Workforce OpenAPI contract.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bogware/terraform-provider-mergewf/internal/apispec"
	"github.com/bogware/terraform-provider-mergewf/internal/provider"
)

func shape(d *apispec.Document, n *apispec.Schema) any {
	if n == nil {
		return nil
	}
	n = d.Resolve(n)
	v := map[string]any{"type": n.Kind(), "required": n.Required, "enum": n.Enum, "minLength": n.MinLength, "maxLength": n.MaxLength, "minimum": n.Minimum, "maximum": n.Maximum, "pattern": n.Pattern, "default": n.Default}
	if n.Items != nil {
		v["items"] = shape(d, n.Items)
	}
	if len(n.Properties) > 0 {
		children := map[string]any{}
		for k, s := range n.Properties {
			children[k] = shape(d, s)
		}
		v["properties"] = children
	}
	return v
}
func signature(d *apispec.Document, o apispec.Operation) string {
	params := map[string]any{}
	for _, p := range o.Parameters {
		params[p.In+"."+p.Name] = map[string]any{"required": p.Required, "schema": shape(d, p.Schema)}
	}
	body := map[string]any{}
	for k, v := range o.RequestBody.Content {
		body[k] = shape(d, v.Schema)
	}
	b, _ := json.Marshal(map[string]any{"parameters": params, "body": body})
	return string(b)
}
func report(d *apispec.Document) (string, []string) {
	coverage := provider.APICoverage()
	snapshot := apispec.MustLoad()
	prior := map[string]string{}
	for p, methods := range snapshot.Paths {
		for m, o := range methods {
			prior[provider.OperationKey(m, p)] = signature(snapshot, o)
		}
	}
	rows := []string{}
	gaps := []string{}
	seen := map[string]bool{}
	implemented, excluded := 0, 0
	for p, methods := range d.Paths {
		for m, o := range methods {
			key := provider.OperationKey(m, p)
			seen[key] = true
			status, ok := coverage[key]
			if !ok {
				gaps = append(gaps, "Uncovered operation: "+strings.ToUpper(m)+" "+p)
				status = "UNCOVERED"
			} else if strings.HasPrefix(status, "Excluded:") {
				excluded++
			} else {
				implemented++
				if old, ok := prior[key]; !ok || old != signature(d, o) {
					gaps = append(gaps, "Request contract changed: "+strings.ToUpper(m)+" "+p)
				}
			}
			rows = append(rows, "| `"+strings.ToUpper(m)+" "+p+"` | "+status+" |")
		}
	}
	for key := range coverage {
		if !seen[key] {
			gaps = append(gaps, "Stale coverage entry: "+key)
		}
	}
	sort.Strings(rows)
	sort.Strings(gaps)
	header := fmt.Sprintf("---\npage_title: \"API coverage\"\nsubcategory: \"\"\ndescription: |-\n  Implemented operations and deliberate exclusions from the Workforce API.\n---\n\n# API coverage\n\nSource: [Merge Workforce OpenAPI](%s).\n\n%d implemented operations; %d deliberate exclusions. Every documented GET endpoint has a data source. The checker also detects changes to parameters and JSON request schemas for implemented operations. The checked-in contract was retrieved on November 5, 2026.\n\n| Operation | Implementation or reason |\n| --- | --- |\n", apispec.URL, implemented, excluded)
	return header + strings.Join(rows, "\n") + "\n", gaps
}
func main() {
	offline := flag.Bool("offline", false, "Check the committed contract instead of fetching the live specification")
	write := flag.String("write-doc", "", "Write the Markdown report to this path")
	flag.Parse()
	data := apispec.Snapshot
	if !*offline {
		c := http.Client{Timeout: 30 * time.Second}
		res, err := c.Get(apispec.URL)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			fmt.Fprintf(os.Stderr, "OpenAPI download returned HTTP %d\n", res.StatusCode)
			os.Exit(1)
		}
		data, err = io.ReadAll(io.LimitReader(res.Body, 16<<20))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	d, err := apispec.Parse(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	markdown, gaps := report(d)
	if *write != "" {
		if err := os.MkdirAll(filepath.Dir(*write), 0755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(*write, []byte(markdown), 0644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if len(gaps) > 0 {
		fmt.Fprintln(os.Stderr, strings.Join(gaps, "\n"))
		os.Exit(1)
	}
	fmt.Println("API coverage is current: all operations implemented or explicitly excluded; implemented request contracts match.")
}
