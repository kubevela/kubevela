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
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ProvisionerACK is the AWS Controllers for Kubernetes.
	ProvisionerACK = "ack"
	// ProvisionerCrossplane is a Crossplane AWS provider.
	ProvisionerCrossplane = "crossplane"

	// maxProvisionerCRDs bounds the CRDs read in full for one service.
	maxProvisionerCRDs = 30
	// maxSchemaBytes bounds the spec schema returned for one kind.
	maxSchemaBytes = 128 << 10
)

var serviceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// provisionerFamily describes how one provisioner names its API groups.
// Families are tried in order: ACK first, then Crossplane.
type provisionerFamily struct {
	name string
	// suffixes are the group suffixes after "<service>.", most current first.
	suffixes []string
	install  string
	health   string
}

var provisionerFamilies = []provisionerFamily{
	{
		name:     ProvisionerACK,
		suffixes: []string{".services.k8s.aws"},
		install:  "the ACK %[1]s controller, chart oci://public.ecr.aws/aws-controllers-k8s/%[1]s-chart",
		health:   "condition ACK.ResourceSynced is True; status.ackResourceMetadata.arn identifies the AWS resource",
	},
	{
		name: ProvisionerCrossplane,
		// Namespaced Crossplane v2 resources, then cluster-scoped, then the
		// older community provider.
		suffixes: []string{".aws.m.upbound.io", ".aws.upbound.io", ".aws.crossplane.io"},
		install:  "a Crossplane AWS provider for %[1]s, provider-aws-%[1]s from crossplane-contrib/provider-upjet-aws",
		health:   "conditions Ready and Synced are True; status.atProvider holds what AWS reported",
	},
}

// CloudProvisioners lists the AWS services each provisioner serves on the
// cluster, read from the installed CRDs' API groups.
type CloudProvisioners struct {
	ACK        []string `json:"ack" jsonschema:"AWS services with ACK CRDs installed, such as dynamodb or s3."`
	Crossplane []string `json:"crossplane" jsonschema:"AWS services with Crossplane provider CRDs installed."`
}

// ProvisionedKind is one resource kind a provisioner offers for a service.
type ProvisionedKind struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion" jsonschema:"Group and storage version, as a definition's output must use."`
	Scope      string `json:"scope" jsonschema:"Namespaced or Cluster."`
}

// ProvisionedResource is the schema of the kind asked for.
type ProvisionedResource struct {
	ProvisionedKind
	// Required lists the spec fields the CRD requires.
	Required []string `json:"required" jsonschema:"Spec fields the CRD requires."`
	// Spec is the storage version's OpenAPI schema for spec.
	Spec map[string]any `json:"spec" jsonschema:"OpenAPI schema of spec from the installed CRD. Build definition parameters only from these fields."`
	// Health says how the provisioner reports readiness.
	Health string `json:"health" jsonschema:"How this provisioner reports that the resource is ready, for the definition's health policy."`
}

// ServiceProvisioner answers which provisioner can create a service here.
type ServiceProvisioner struct {
	Service string `json:"service"`
	// Provisioner is ack or crossplane, or empty when neither is installed.
	Provisioner string            `json:"provisioner" jsonschema:"ack or crossplane: the provisioner to build on, ACK first when both are installed. Empty when neither is installed."`
	Kinds       []ProvisionedKind `json:"kinds" jsonschema:"Kinds that provisioner offers for the service."`
	// Also lists other provisioners found for the same service.
	Also     []string             `json:"also,omitempty" jsonschema:"Other provisioners that also serve this service on the cluster."`
	Resource *ProvisionedResource `json:"resource,omitempty" jsonschema:"Schema of the kind asked for."`
	// Message is set when no provisioner was found or a kind is missing.
	Message string `json:"message,omitempty" jsonschema:"Why the service cannot be provisioned yet and what to install. Relay it to the user and do not write a definition for a provisioner that is not installed."`
}

// cloudProvisioners lists CRD names only, then groups them by family.
func cloudProvisioners(ctx context.Context, cli client.Client) (*CloudProvisioners, []string, error) {
	names, err := crdNames(ctx, cli)
	if err != nil {
		return nil, nil, err
	}
	found := map[string]map[string]bool{ProvisionerACK: {}, ProvisionerCrossplane: {}}
	for _, n := range names {
		_, group, ok := strings.Cut(n, ".")
		if !ok {
			continue
		}
		for _, fam := range provisionerFamilies {
			for _, suf := range fam.suffixes {
				if svc, ok := strings.CutSuffix(group, suf); ok && svc != "" && !strings.Contains(svc, ".") {
					found[fam.name][svc] = true
				}
			}
		}
	}
	return &CloudProvisioners{ACK: sortedKeys(found[ProvisionerACK]), Crossplane: sortedKeys(found[ProvisionerCrossplane])}, names, nil
}

// serviceProvisioner resolves the provisioner for one service from the CRD
// names already listed, reads the matching CRDs, and fills the schema of
// kind when given.
func serviceProvisioner(ctx context.Context, cli client.Client, names []string, service, kind string) (*ServiceProvisioner, []string, error) {
	out := &ServiceProvisioner{Service: service, Kinds: []ProvisionedKind{}}
	var warnings []string
	type match struct {
		family provisionerFamily
		rank   int
		name   string
	}
	var matches []match
	for _, n := range names {
		_, group, ok := strings.Cut(n, ".")
		if !ok {
			continue
		}
		for _, fam := range provisionerFamilies {
			for rank, suf := range fam.suffixes {
				if group == service+suf {
					matches = append(matches, match{fam, rank, n})
				}
			}
		}
	}
	byFamily := map[string][]match{}
	for _, m := range matches {
		byFamily[m.family.name] = append(byFamily[m.family.name], m)
	}
	var chosen *provisionerFamily
	for i := range provisionerFamilies {
		fam := provisionerFamilies[i]
		if len(byFamily[fam.name]) == 0 {
			continue
		}
		if chosen == nil {
			chosen = &fam
		} else {
			out.Also = append(out.Also, fam.name)
		}
	}
	if chosen == nil {
		out.Message = noProvisionerMessage(service)
		return out, warnings, nil
	}
	out.Provisioner = chosen.name

	// Prefer the most current group of the family, then read its CRDs.
	fm := byFamily[chosen.name]
	best := len(chosen.suffixes)
	for _, m := range fm {
		if m.rank < best {
			best = m.rank
		}
	}
	var crds []*crdv1.CustomResourceDefinition
	for _, m := range fm {
		if m.rank != best {
			continue
		}
		if len(crds) == maxProvisionerCRDs {
			warnings = append(warnings, fmt.Sprintf("more than %d %s CRDs for %s; only the first %d are listed", maxProvisionerCRDs, chosen.name, service, maxProvisionerCRDs))
			break
		}
		crd := &crdv1.CustomResourceDefinition{}
		if err := cli.Get(ctx, client.ObjectKey{Name: m.name}, crd); err != nil {
			warnings = append(warnings, fmt.Sprintf("CustomResourceDefinition %s could not be read: %s", m.name, redactLine(err.Error())))
			continue
		}
		crds = append(crds, crd)
	}
	sort.Slice(crds, func(i, j int) bool { return crds[i].Spec.Names.Kind < crds[j].Spec.Names.Kind })
	for _, crd := range crds {
		if v := storageVersion(crd); v != nil {
			out.Kinds = append(out.Kinds, ProvisionedKind{Kind: crd.Spec.Names.Kind, APIVersion: crd.Spec.Group + "/" + v.Name, Scope: string(crd.Spec.Scope)})
		}
	}
	if kind == "" {
		return out, warnings, nil
	}
	for _, crd := range crds {
		if !strings.EqualFold(crd.Spec.Names.Kind, kind) {
			continue
		}
		res, w, err := provisionedResource(crd, chosen.health)
		if err != nil {
			return nil, warnings, err
		}
		out.Resource = res
		return out, append(warnings, w...), nil
	}
	kinds := make([]string, 0, len(out.Kinds))
	for _, k := range out.Kinds {
		kinds = append(kinds, k.Kind)
	}
	out.Message = fmt.Sprintf("%s serves %s on this cluster, but has no %s kind; the installed kinds are %s", chosen.name, service, kind, strings.Join(kinds, ", "))
	return out, warnings, nil
}

func noProvisionerMessage(service string) string {
	var groups, installs []string
	for _, fam := range provisionerFamilies {
		var gs []string
		for _, suf := range fam.suffixes {
			gs = append(gs, service+suf)
		}
		groups = append(groups, fmt.Sprintf("%s (%s)", fam.name, strings.Join(gs, ", ")))
		installs = append(installs, fmt.Sprintf(fam.install, service))
	}
	return fmt.Sprintf("Neither ACK nor Crossplane provides %s on this cluster: no CRDs were found in the groups of %s. "+
		"Creating %s resources from KubeVela needs one of them: install %s, or %s. "+
		"Ask the platform team which provisioner this cluster should use; a definition for a provisioner that is not installed cannot be checked or applied.",
		service, strings.Join(groups, " or "), service, installs[0], installs[1])
}

// provisionedResource extracts the storage version's spec schema, dropping
// descriptions when the schema is too large to return whole.
func provisionedResource(crd *crdv1.CustomResourceDefinition, health string) (*ProvisionedResource, []string, error) {
	v := storageVersion(crd)
	res := &ProvisionedResource{
		ProvisionedKind: ProvisionedKind{Kind: crd.Spec.Names.Kind, APIVersion: crd.Spec.Group + "/" + v.Name, Scope: string(crd.Spec.Scope)},
		Required:        []string{},
		Spec:            map[string]any{},
		Health:          health,
	}
	if v.Schema == nil || v.Schema.OpenAPIV3Schema == nil {
		return res, []string{fmt.Sprintf("CustomResourceDefinition %s publishes no schema", crd.Name)}, nil
	}
	spec, ok := v.Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		return res, []string{fmt.Sprintf("CustomResourceDefinition %s has no spec in its schema", crd.Name)}, nil
	}
	res.Required = append(res.Required, spec.Required...)
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, err
	}
	if len(raw) > maxSchemaBytes {
		dropDescriptions(m)
		if trimmed, _ := json.Marshal(m); len(trimmed) > maxSchemaBytes {
			m = topLevelOnly(m)
			warnings = append(warnings, fmt.Sprintf("the %s spec schema is larger than %d KiB; only top-level fields and their types are returned", res.Kind, maxSchemaBytes>>10))
		} else {
			warnings = append(warnings, fmt.Sprintf("the %s spec schema is larger than %d KiB; field descriptions were dropped", res.Kind, maxSchemaBytes>>10))
		}
	}
	res.Spec = m
	return res, warnings, nil
}

func storageVersion(crd *crdv1.CustomResourceDefinition) *crdv1.CustomResourceDefinitionVersion {
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Storage {
			return &crd.Spec.Versions[i]
		}
	}
	if len(crd.Spec.Versions) > 0 {
		return &crd.Spec.Versions[0]
	}
	return &crdv1.CustomResourceDefinitionVersion{}
}

func dropDescriptions(v any) {
	switch t := v.(type) {
	case map[string]any:
		delete(t, "description")
		for _, c := range t {
			dropDescriptions(c)
		}
	case []any:
		for _, c := range t {
			dropDescriptions(c)
		}
	}
}

func topLevelOnly(spec map[string]any) map[string]any {
	out := map[string]any{"type": spec["type"]}
	if req, ok := spec["required"]; ok {
		out["required"] = req
	}
	props, _ := spec["properties"].(map[string]any)
	short := map[string]any{}
	for name, p := range props {
		if pm, ok := p.(map[string]any); ok {
			short[name] = map[string]any{"type": pm["type"]}
		}
	}
	out["properties"] = short
	return out
}

// crdNames lists CustomResourceDefinition names through the metadata API,
// so no schema is downloaded until a service is chosen.
func crdNames(ctx context.Context, cli client.Client) ([]string, error) {
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(crdv1.SchemeGroupVersion.WithKind("CustomResourceDefinitionList"))
	if err := cli.List(ctx, list); err != nil {
		return nil, fmt.Errorf("list CustomResourceDefinitions: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for i := range list.Items {
		names = append(names, list.Items[i].Name)
	}
	sort.Strings(names)
	return names, nil
}
