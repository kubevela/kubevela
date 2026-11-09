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

package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// defHandlePrefix marks a staged-definition handle in a definitions entry.
const defHandlePrefix = "def:sha256:"

var (
	// stageTTL is how long a staged definition stays usable. Long enough for
	// an authoring loop between edits, short enough that a stale draft does
	// not survive into the next session. A var so tests can lower it.
	stageTTL = time.Hour
	// maxStagedDefinitions bounds the in-memory stage cache.
	maxStagedDefinitions = 64
)

// stagedDefinition is one parsed definition held for reuse by handle.
type stagedDefinition struct {
	obj *unstructured.Unstructured
	// size is the staged content's byte length, so handle entries count
	// toward the same aggregate budget as inline ones.
	size      int
	createdAt time.Time
}

// stageCache holds staged definitions for stageTTL, bounded in count.
type stageCache struct {
	entries map[string]stagedDefinition
}

func (c *stageCache) put(handle string, obj *unstructured.Unstructured, size int) {
	if c.entries == nil {
		c.entries = map[string]stagedDefinition{}
	}
	now := time.Now()
	for h, e := range c.entries {
		if now.Sub(e.createdAt) > stageTTL {
			delete(c.entries, h)
		}
	}
	if _, exists := c.entries[handle]; !exists && len(c.entries) >= maxStagedDefinitions {
		var oldest string
		for h, e := range c.entries {
			if oldest == "" || e.createdAt.Before(c.entries[oldest].createdAt) {
				oldest = h
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[handle] = stagedDefinition{obj: obj, size: size, createdAt: now}
}

// get returns a copy of the staged definition and its content size for a
// live handle, or false.
func (c *stageCache) get(handle string) (*unstructured.Unstructured, int, bool) {
	e, ok := c.entries[handle]
	if !ok || time.Since(e.createdAt) > stageTTL {
		return nil, 0, false
	}
	return e.obj.DeepCopy(), e.size, true
}

// StageDefinitionInput carries the definition to stage.
type StageDefinitionInput struct {
	Definition string `json:"definition" jsonschema:"The definition to stage, as CUE the way vela def gen-module writes it for a defkit module, or as a definition object in YAML. Validated exactly as the plan tools validate inline definitions."`
}

// StageDefinitionOutput names what was staged and the handle to reuse it by.
type StageDefinitionOutput struct {
	// Handle stands in for the definition in every plan tool.
	Handle     string    `json:"handle" jsonschema:"Content-hash handle the plan tools accept wherever they accept an inline definition. The same content stages to the same handle."`
	Kind       string    `json:"kind" jsonschema:"Definition kind that was staged, such as ComponentDefinition."`
	Name       string    `json:"name" jsonschema:"Name of the staged definition."`
	APIVersion string    `json:"apiVersion" jsonschema:"API version of the staged definition object."`
	ExpiresAt  time.Time `json:"expiresAt" jsonschema:"When the handle stops resolving, UTC. Stage the definition again after an edit or after expiry."`
	Evidence   []string  `json:"evidence" jsonschema:"What was parsed and staged."`
	Meta       Meta      `json:"meta"`
}

func (d *deps) stageDefinition(ctx context.Context, _ *sdk.CallToolRequest, in StageDefinitionInput) (*sdk.CallToolResult, StageDefinitionOutput, error) {
	out := StageDefinitionOutput{Evidence: []string{}}
	if strings.TrimSpace(in.Definition) == "" {
		return nil, out, fmt.Errorf("definition is required")
	}
	if len(in.Definition) > maxDefinitionBytes {
		return nil, out, fmt.Errorf("definition is %d bytes; at most %d are accepted", len(in.Definition), maxDefinitionBytes)
	}
	obj, err := parseDefinition(in.Definition)
	if err != nil {
		return nil, out, fmt.Errorf("definition does not parse: %w", err)
	}
	if obj.GetKind() == "" || obj.GetName() == "" {
		return nil, out, fmt.Errorf("definition needs kind and metadata.name")
	}
	if !strings.HasSuffix(obj.GetKind(), "Definition") {
		return nil, out, fmt.Errorf("definition is a %s, only definition kinds are accepted", obj.GetKind())
	}
	sum := sha256.Sum256([]byte(in.Definition))
	out.Handle = defHandlePrefix + hex.EncodeToString(sum[:])
	out.Kind, out.Name, out.APIVersion = obj.GetKind(), obj.GetName(), obj.GetAPIVersion()
	d.mu.Lock()
	d.stages.put(out.Handle, obj, len(in.Definition))
	d.mu.Unlock()
	out.ExpiresAt = time.Now().Add(stageTTL).UTC()
	out.Evidence = append(out.Evidence, fmt.Sprintf("parsed and staged %s %q from %d bytes; nothing was sent to the cluster", out.Kind, out.Name, len(in.Definition)))
	out.Meta = d.meta(ctx)
	out.Meta.Source = SourceRendered
	return nil, out, nil
}

// resolveStagedDefinition returns the staged object when entry is a handle.
// The second result says whether entry was a handle at all, so callers fall
// back to inline parsing when it was not.
func (d *deps) resolveStagedDefinition(entry string) (*unstructured.Unstructured, int, bool, error) {
	if !strings.HasPrefix(entry, defHandlePrefix) {
		return nil, 0, false, nil
	}
	d.mu.Lock()
	obj, size, ok := d.stages.get(entry)
	d.mu.Unlock()
	if !ok {
		return nil, 0, true, fmt.Errorf("handle %s is not staged or has expired; stage the definition again with stage_definition", entry)
	}
	return obj, size, true, nil
}
