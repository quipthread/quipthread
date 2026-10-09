// Package catalogfile renders packages/plans/catalog.json, the single source
// of plan data, as the Go source of plans/catalog_gen.go.
package catalogfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"sort"
)

// SourcePath is catalog.json relative to the backend/plans directory.
const SourcePath = "../../packages/plans/catalog.json"

type catalog struct {
	Plans []struct {
		ID     string `json:"id"`
		Rank   int    `json:"rank"`
		Limits struct {
			Comments int `json:"comments"`
			Sites    int `json:"sites"`
		} `json:"limits"`
	} `json:"plans"`
}

// Render returns gofmt-ed Go source declaring the plan catalog.
func Render(source []byte) ([]byte, error) {
	var c catalog
	dec := json.NewDecoder(bytes.NewReader(source))
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if len(c.Plans) == 0 {
		return nil, fmt.Errorf("catalog has no plans")
	}
	sort.Slice(c.Plans, func(i, j int) bool { return c.Plans[i].Rank < c.Plans[j].Rank })

	var b bytes.Buffer
	b.WriteString("// Code generated from packages/plans/catalog.json by ./internal/gencatalog. DO NOT EDIT.\n\n")
	b.WriteString("package plans\n\n")
	b.WriteString("var catalog = map[string]Limits{\n")
	seen := map[string]bool{}
	for _, p := range c.Plans {
		if p.ID == "" || seen[p.ID] {
			return nil, fmt.Errorf("catalog: missing or duplicate plan id %q", p.ID)
		}
		seen[p.ID] = true
		fmt.Fprintf(&b, "\t%q: {Comments: %d, Sites: %d, Rank: %d},\n", p.ID, p.Limits.Comments, p.Limits.Sites, p.Rank)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}
