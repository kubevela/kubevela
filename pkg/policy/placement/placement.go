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

// Package placement classifies objects as hub or runtime and builds the
// topology policies, override policies, and deploy steps that install each
// class. It does not know what a module is. Callers name the tiers and pass
// the objects.
package placement

import (
	"fmt"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const hubSuffix = "-hub"

// Component is one already-named tier and the class it belongs to.
type Component struct {
	Name  string
	Class string
}

// Group is the objects of one class split out of a single tier.
type Group struct {
	Name    string
	Class   string
	Objects []map[string]interface{}
}

// Names are the policy and step names the caller wants on the Application.
// Empty fields are omitted from the result rather than invented.
type Names struct {
	HubPolicy       string
	RuntimePolicy   string
	HubOverride     string
	RuntimeOverride string
	HubStep         string
	RuntimeStep     string
}

// Class reads oam.AnnotationPlacement on obj, then the first non-empty inherited default.
// The caller passes defaults in precedence order (line, then module). An
// absent marker with no default is hub. Any other value is an error.
func Class(obj map[string]interface{}, inherited ...string) (string, error) {
	v, ok, err := annotation(obj)
	if err != nil {
		return "", err
	}
	if ok {
		return parseClass(v, obj)
	}
	for _, d := range inherited {
		if d == "" {
			continue
		}
		return parseClass(d, obj)
	}
	return oam.PlacementHub, nil
}

// Marked reports whether obj carries an explicit placement annotation.
// An absent annotation is not marked. An invalid value is an error.
func Marked(obj map[string]interface{}) (bool, error) {
	v, ok, err := annotation(obj)
	if err != nil || !ok {
		return false, err
	}
	if _, err = parseClass(v, obj); err != nil {
		return false, err
	}
	return true, nil
}

// SplitAux divides objects into the classes actually present, preserving
// source order inside each class. The runtime group keeps base as its name.
// The hub group is base plus "-hub". An empty input returns no groups.
func SplitAux(base string, objects []map[string]interface{}, inherited ...string) ([]Group, error) {
	var runtime, hub []map[string]interface{}
	for _, obj := range objects {
		class, err := Class(obj, inherited...)
		if err != nil {
			return nil, err
		}
		if class == oam.PlacementHub {
			hub = append(hub, obj)
			continue
		}
		runtime = append(runtime, obj)
	}
	var groups []Group
	if len(runtime) > 0 {
		groups = append(groups, Group{Name: base, Class: oam.PlacementRuntime, Objects: runtime})
	}
	if len(hub) > 0 {
		groups = append(groups, Group{Name: base + hubSuffix, Class: oam.PlacementHub, Objects: hub})
	}
	return groups, nil
}

// Assemble builds the policies and the two deploy steps for components.
// Hub components always bind to the hub topology, which selects the local
// cluster. Runtime components bind to the runtime topology when req is
// non-nil, and to the hub topology otherwise. Both steps are always returned.
// req's namespace is not copied: placement here chooses clusters, not a
// namespace.
func Assemble(components []Component, req *v1alpha1.Placement, names Names) ([]map[string]interface{}, []map[string]interface{}) {
	var hubNames, runtimeNames []string
	for _, c := range components {
		if c.Class == oam.PlacementHub {
			hubNames = append(hubNames, c.Name)
			continue
		}
		runtimeNames = append(runtimeNames, c.Name)
	}

	hubPlace := v1alpha1.Placement{Clusters: []string{pkgmulticluster.Local}}
	policies := []map[string]interface{}{
		policy(names.HubPolicy, v1alpha1.TopologyPolicyType, topologyProps(hubPlace)),
		policy(names.HubOverride, v1alpha1.OverridePolicyType, map[string]interface{}{
			"selector": stringList(hubNames),
		}),
	}
	runtimeTopology := names.HubPolicy
	if req != nil {
		runtimeTopology = names.RuntimePolicy
		policies = append(policies, policy(names.RuntimePolicy, v1alpha1.TopologyPolicyType, topologyProps(*req)))
	}
	policies = append(policies, policy(names.RuntimeOverride, v1alpha1.OverridePolicyType, map[string]interface{}{
		"selector": stringList(runtimeNames),
	}))

	steps := []map[string]interface{}{
		deployStep(names.HubStep, []string{names.HubPolicy, names.HubOverride}),
		deployStep(names.RuntimeStep, []string{runtimeTopology, names.RuntimeOverride}),
	}
	return policies, steps
}

func policy(name, typ string, properties map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"name":       name,
		"type":       typ,
		"properties": properties,
	}
}

func deployStep(name string, policies []string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"type": "deploy",
		"properties": map[string]interface{}{
			"policies": stringList(policies),
		},
	}
}

func topologyProps(p v1alpha1.Placement) map[string]interface{} {
	props := map[string]interface{}{}
	if len(p.Clusters) > 0 {
		props["clusters"] = stringList(p.Clusters)
	}
	if len(p.ClusterLabelSelector) > 0 {
		sel := make(map[string]interface{}, len(p.ClusterLabelSelector))
		for k, v := range p.ClusterLabelSelector {
			sel[k] = v
		}
		props["clusterLabelSelector"] = sel
	}
	if p.AllowEmpty {
		props["allowEmpty"] = true
	}
	return props
}

func stringList(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func annotation(obj map[string]interface{}) (string, bool, error) {
	meta, _ := obj["metadata"].(map[string]interface{})
	if meta == nil {
		return "", false, nil
	}
	annos, _ := meta["annotations"].(map[string]interface{})
	if annos == nil {
		return "", false, nil
	}
	v, ok := annos[oam.AnnotationPlacement]
	if !ok || v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("placement class on %s is not a string", objectName(obj))
	}
	return s, true, nil
}

// Normalize checks a placement value from a module or line default.
// Hub and runtime are accepted. An empty value and any other value are errors.
func Normalize(v string) (string, error) {
	if err := oam.ValidatePlacement(v); err != nil {
		return "", err
	}
	return v, nil
}

func parseClass(v string, obj map[string]interface{}) (string, error) {
	if err := oam.ValidatePlacement(v); err != nil {
		return "", fmt.Errorf("placement on %s: %w", objectName(obj), err)
	}
	return v, nil
}

func objectName(obj map[string]interface{}) string {
	meta, _ := obj["metadata"].(map[string]interface{})
	if meta == nil {
		return "object"
	}
	name, _ := meta["name"].(string)
	if name == "" {
		return "object"
	}
	return name
}
