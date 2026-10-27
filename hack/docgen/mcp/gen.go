/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Command gen writes the vela mcp tool reference for kubevela.io. It starts
// the server in memory with every tool registered and reads the tools the
// way a client does, so the page always matches what the binary serves.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/mcp"
)

func main() {
	path := "../kubevela.github.io/docs/reference/vela-mcp-tools.md"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	tools, err := listTools(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	page, err := render(tools)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote", path)
}

// tool is one tool as a client sees it, plus the class the server gave it.
type tool struct {
	*sdk.Tool
	Class mcp.ToolClass
}

// listTools starts a server with every optional tool enabled and lists its
// tools over an in-memory session.
func listTools(ctx context.Context) ([]tool, error) {
	srv, err := mcp.NewServer(velacmd.NewTestFactory(nil, nil), mcp.Options{AllowWrite: true, AllowResume: true})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	clientT, serverT := sdk.NewInMemoryTransports()
	go func() { _ = srv.Serve(ctx, serverT) }()
	session, err := sdk.NewClient(&sdk.Implementation{Name: "docgen"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = session.Close() }()
	listed := map[string]*sdk.Tool{}
	for t, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		listed[t.Name] = t
	}
	// Registration order follows the order an agent works in, from platform
	// info to apply, which reads better than the alphabetical listing.
	var out []tool
	for _, r := range srv.Tools() {
		t, ok := listed[r.Name]
		if !ok {
			return nil, fmt.Errorf("tool %s was registered but not listed", r.Name)
		}
		out = append(out, tool{Tool: t, Class: r.Class})
	}
	return out, nil
}

// schema is the part of a JSON Schema the page shows.
type schema struct {
	Type        any                `json:"type"`
	Description string             `json:"description"`
	Items       *schema            `json:"items"`
	Required    []string           `json:"required"`
	Properties  map[string]*schema `json:"properties"`
}

// keys lists properties sorted by name, with required ones first when
// requiredFirst is set. The client sees properties as a JSON object, so
// declaration order is lost.
func (s *schema) keys(requiredFirst bool) []string {
	required := map[string]bool{}
	for _, r := range s.Required {
		required[r] = true
	}
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if requiredFirst && required[keys[i]] != required[keys[j]] {
			return required[keys[i]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// glossary describes fields that many results share and that the server
// leaves undescribed because their meaning is the same everywhere.
var glossary = map[string]string{
	"meta":        "Versions and read time for this result. See [the meta block](#the-meta-block).",
	"evidence":    "Objects this answer was derived from.",
	"name":        "Name of the object this result describes.",
	"namespace":   "Namespace of the object this result describes.",
	"application": "Application name.",
}

func toSchema(v any) (*schema, error) {
	if v == nil {
		return &schema{}, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := &schema{}
	return s, json.Unmarshal(b, s)
}

// typeName prints a schema type the way a reader expects: string, integer,
// array of string, object.
func typeName(s *schema) string {
	var t string
	switch v := s.Type.(type) {
	case string:
		t = v
	case []any:
		var parts []string
		for _, p := range v {
			if p != "null" {
				parts = append(parts, fmt.Sprint(p))
			}
		}
		t = strings.Join(parts, " or ")
	}
	if t == "array" && s.Items != nil {
		return "array of " + typeName(s.Items)
	}
	if t == "" {
		return "any"
	}
	return t
}

// cell makes text safe inside a markdown table cell and for MDX.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.NewReplacer("|", `\|`, "<", "&lt;", ">", "&gt;", "{", `\{`, "}", `\}`).Replace(s)
}

var classText = map[mcp.ToolClass]string{
	mcp.ToolClassRead:  "Read. Always registered.",
	mcp.ToolClassPlan:  "Plan. Always registered; renders and compares without writing.",
	mcp.ToolClassWrite: "Write. Registered only when the server starts with `--allow-write`.",
}

const header = `---
title: vela mcp tool reference
---

This page lists every tool that ` + "`vela mcp`" + ` serves, with its inputs and the top-level
fields of its result. Agents read the same descriptions and full JSON schemas from the
server when they connect, so this page is for people: to see what an agent can do and
what it gets back. To set the server up, see [AI agents with vela mcp](../tutorials/vela-mcp.md).

Tools come in three classes:

- **Read** tools are always registered and never change the cluster.
- **Plan** tools render or compare an Application without saving it.
- **Write** tools exist only when the server is started with a flag that enables them.

`

// render builds the page. The envelope is described once, from the meta
// field of the first tool that returns one.
func render(tools []tool) (string, error) {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("| Tool | Class | Summary |\n|---|---|---|\n")
	for _, t := range tools {
		fmt.Fprintf(&b, "| [`%s`](#%s) | %s | %s |\n", t.Name, t.Name, t.Class, cell(firstSentence(t.Description)))
	}

	var meta *schema
	for _, t := range tools {
		out, err := toSchema(t.OutputSchema)
		if err != nil {
			return "", err
		}
		if m := out.Properties["meta"]; m != nil {
			meta = m
			break
		}
	}
	if meta != nil {
		b.WriteString("\n## The meta block\n\nEvery result carries a `meta` block that says which system the data came from and when it was read.\n\n")
		writeFields(&b, meta, false)
	}

	for _, t := range tools {
		fmt.Fprintf(&b, "\n## %s\n\n", t.Name)
		fmt.Fprintf(&b, "%s\n\n", classText[t.Class])
		fmt.Fprintf(&b, "%s\n\n", cell(t.Description))
		in, err := toSchema(t.InputSchema)
		if err != nil {
			return "", err
		}
		b.WriteString("### Input\n\n")
		if len(in.Properties) == 0 {
			b.WriteString("This tool takes no input.\n\n")
		} else {
			writeFields(&b, in, true)
		}
		out, err := toSchema(t.OutputSchema)
		if err != nil {
			return "", err
		}
		if len(out.Properties) > 0 {
			b.WriteString("### Result\n\n")
			writeFields(&b, out, false)
		}
	}
	b.WriteString("\n###### Auto generated by [hack/docgen/mcp](https://github.com/kubevela/kubevela/tree/master/hack/docgen/mcp) in KubeVela.\n")
	return b.String(), nil
}

func writeFields(b *strings.Builder, s *schema, withRequired bool) {
	required := map[string]bool{}
	for _, r := range s.Required {
		required[r] = true
	}
	if withRequired {
		b.WriteString("| Field | Type | Required | Description |\n|---|---|---|---|\n")
	} else {
		b.WriteString("| Field | Type | Description |\n|---|---|---|\n")
	}
	for _, k := range s.keys(withRequired) {
		p := s.Properties[k]
		desc := p.Description
		if desc == "" {
			desc = glossary[k]
		}
		if withRequired {
			req := "no"
			if required[k] {
				req = "yes"
			}
			fmt.Fprintf(b, "| `%s` | %s | %s | %s |\n", k, typeName(p), req, cell(desc))
		} else {
			fmt.Fprintf(b, "| `%s` | %s | %s |\n", k, typeName(p), cell(desc))
		}
	}
	b.WriteString("\n")
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i >= 0 {
		return s[:i+1]
	}
	return s
}
