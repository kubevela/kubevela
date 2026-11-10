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
	// ProvisionerConfigConnector is GCP's Config Connector.
	ProvisionerConfigConnector = "config-connector"
	// ProvisionerASO is the Azure Service Operator.
	ProvisionerASO = "aso"
	// ProvisionerCrossplane is a Crossplane provider for the chosen cloud.
	ProvisionerCrossplane = "crossplane"

	// Clouds a provisioner can be resolved for.
	ProviderAWS   = "aws"
	ProviderGCP   = "gcp"
	ProviderAzure = "azure"

	// maxProvisionerCRDs bounds the CRDs read in full for one service.
	maxProvisionerCRDs = 30
	// maxSchemaBytes bounds the spec schema returned for one kind.
	maxSchemaBytes = 128 << 10
)

var serviceName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// provisionerFamily describes how one provisioner names its API groups.
// Per cloud, families are tried in order: the cloud's own controller first,
// then Crossplane, matching the platform default definitions build on.
type provisionerFamily struct {
	name string
	// display is the family's proper name for prose messages; name stays the
	// canonical lower-case value the provisioner field reports.
	display string
	// suffixes are the group suffixes after "<service>.", most current first.
	suffixes []string
	install  string
	health   string
}

// providerFamilies maps each cloud to its families, own controller first.
var providerFamilies = map[string][]provisionerFamily{
	ProviderAWS: {
		{
			name:     ProvisionerACK,
			display:  "ACK",
			suffixes: []string{".services.k8s.aws"},
			install:  "the ACK %[1]s controller, chart oci://public.ecr.aws/aws-controllers-k8s/%[1]s-chart",
			health:   "condition ACK.ResourceSynced is True; status.ackResourceMetadata.arn identifies the AWS resource",
		},
		{
			name:    ProvisionerCrossplane,
			display: "Crossplane",
			// Namespaced Crossplane v2 resources, then cluster-scoped, then the
			// older community provider.
			suffixes: []string{".aws.m.upbound.io", ".aws.upbound.io", ".aws.crossplane.io"},
			install:  "a Crossplane AWS provider for %[1]s, provider-aws-%[1]s from crossplane-contrib/provider-upjet-aws",
			health:   "conditions Ready and Synced are True; status.atProvider holds what AWS reported",
		},
	},
	ProviderGCP: {
		{
			name:     ProvisionerConfigConnector,
			display:  "Config Connector",
			suffixes: []string{".cnrm.cloud.google.com"},
			install:  "Config Connector, which serves every GCP service once installed: the ConfigConnector add-on on GKE, or the operator bundle gs://configconnector-operator/latest/release-bundle.tar.gz elsewhere",
			health:   "condition Ready is True with reason UpToDate; the resource's status holds what GCP reported",
		},
		{
			name:     ProvisionerCrossplane,
			display:  "Crossplane",
			suffixes: []string{".gcp.m.upbound.io", ".gcp.upbound.io", ".gcp.crossplane.io"},
			install:  "a Crossplane GCP provider for %[1]s, provider-gcp-%[1]s from crossplane-contrib/provider-upjet-gcp",
			health:   "conditions Ready and Synced are True; status.atProvider holds what GCP reported",
		},
	},
	ProviderAzure: {
		{
			name:     ProvisionerASO,
			display:  "Azure Service Operator",
			suffixes: []string{".azure.com"},
			install:  "Azure Service Operator with the %[1]s CRDs: chart azure-service-operator from https://raw.githubusercontent.com/Azure/azure-service-operator/main/v2/charts with --set crdPattern='%[1]s.azure.com/*'",
			health:   "condition Ready is True, meaning the resource reached its goal state; a Ready that is not True carries a severity and a message",
		},
		{
			name:     ProvisionerCrossplane,
			display:  "Crossplane",
			suffixes: []string{".azure.m.upbound.io", ".azure.upbound.io", ".azure.crossplane.io"},
			install:  "a Crossplane Azure provider for %[1]s, provider-azure-%[1]s from crossplane-contrib/provider-upjet-azure",
			health:   "conditions Ready and Synced are True; status.atProvider holds what Azure reported",
		},
	},
}

// providerOrder keeps cloud listings deterministic.
var providerOrder = []string{ProviderAWS, ProviderGCP, ProviderAzure}

// formatInstall fills the service into an install hint that takes one; hints
// for monolithic controllers, which install every service at once, do not.
func formatInstall(install, service string) string {
	if strings.Contains(install, "%") {
		return fmt.Sprintf(install, service)
	}
	return install
}

// AWSProvisioners lists the AWS services each family serves here.
type AWSProvisioners struct {
	ACK        []string `json:"ack" jsonschema:"AWS services with ACK CRDs installed, such as dynamodb or s3."`
	Crossplane []string `json:"crossplane" jsonschema:"AWS services with Crossplane provider CRDs installed."`
}

// GCPProvisioners lists the GCP services each family serves here.
type GCPProvisioners struct {
	ConfigConnector []string `json:"configConnector" jsonschema:"GCP services with Config Connector CRDs installed, such as sql or storage."`
	Crossplane      []string `json:"crossplane" jsonschema:"GCP services with Crossplane provider CRDs installed."`
}

// AzureProvisioners lists the Azure services each family serves here.
type AzureProvisioners struct {
	AzureServiceOperator []string `json:"azureServiceOperator" jsonschema:"Azure services with Azure Service Operator CRDs installed, such as sql or storage."`
	Crossplane           []string `json:"crossplane" jsonschema:"Azure services with Crossplane provider CRDs installed."`
}

// CloudProvisioners lists the cloud services each provisioner serves on the
// cluster, read from the installed CRDs' API groups.
type CloudProvisioners struct {
	AWS   AWSProvisioners   `json:"aws" jsonschema:"AWS services served by ACK and Crossplane on this cluster."`
	GCP   GCPProvisioners   `json:"gcp" jsonschema:"GCP services served by Config Connector and Crossplane on this cluster."`
	Azure AzureProvisioners `json:"azure" jsonschema:"Azure services served by Azure Service Operator and Crossplane on this cluster."`
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
	// Provider is the cloud the service was resolved for.
	Provider string `json:"provider" jsonschema:"Cloud the service was resolved for: aws, gcp or azure."`
	// Provisioner names the family to build on, or is empty when none is installed.
	Provisioner string            `json:"provisioner" jsonschema:"ack, config-connector, aso or crossplane: the provisioner to build on, the cloud's own controller first when both are installed. Empty when neither is installed."`
	Kinds       []ProvisionedKind `json:"kinds" jsonschema:"Kinds that provisioner offers for the service."`
	// Also lists other provisioners found for the same service.
	Also     []string             `json:"also,omitempty" jsonschema:"Other provisioners that also serve this service on the cluster."`
	Resource *ProvisionedResource `json:"resource,omitempty" jsonschema:"Schema of the kind asked for."`
	// Message is set when no provisioner was found or a kind is missing.
	Message string `json:"message,omitempty" jsonschema:"Why the service cannot be provisioned yet and what to install. Relay it to the user and do not write a definition for a provisioner that is not installed."`

	// truncated is set when a CRD or schema budget cut the data.
	truncated bool
}

// cloudProvisioners lists CRD names only, then groups them by cloud and family.
func cloudProvisioners(ctx context.Context, cli client.Client) (*CloudProvisioners, []string, error) {
	names, err := crdNames(ctx, cli)
	if err != nil {
		return nil, nil, err
	}
	type key struct{ provider, family string }
	found := map[key]map[string]bool{}
	for _, n := range names {
		_, group, ok := strings.Cut(n, ".")
		if !ok {
			continue
		}
		for _, provider := range providerOrder {
			for _, fam := range providerFamilies[provider] {
				for _, suf := range fam.suffixes {
					if svc, ok := strings.CutSuffix(group, suf); ok && svc != "" && !strings.Contains(svc, ".") {
						k := key{provider, fam.name}
						if found[k] == nil {
							found[k] = map[string]bool{}
						}
						found[k][svc] = true
					}
				}
			}
		}
	}
	return &CloudProvisioners{
		AWS:   AWSProvisioners{ACK: sortedKeys(found[key{ProviderAWS, ProvisionerACK}]), Crossplane: sortedKeys(found[key{ProviderAWS, ProvisionerCrossplane}])},
		GCP:   GCPProvisioners{ConfigConnector: sortedKeys(found[key{ProviderGCP, ProvisionerConfigConnector}]), Crossplane: sortedKeys(found[key{ProviderGCP, ProvisionerCrossplane}])},
		Azure: AzureProvisioners{AzureServiceOperator: sortedKeys(found[key{ProviderAzure, ProvisionerASO}]), Crossplane: sortedKeys(found[key{ProviderAzure, ProvisionerCrossplane}])},
	}, names, nil
}

// serviceProvisioner resolves the provisioner for one service from the CRD
// names already listed, reads the matching CRDs, and fills the schema of
// kind when given.
func serviceProvisioner(ctx context.Context, cli client.Client, names []string, provider, service, kind string) (*ServiceProvisioner, []string, error) {
	families := providerFamilies[provider]
	out := &ServiceProvisioner{Service: service, Provider: provider, Kinds: []ProvisionedKind{}}
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
		for _, fam := range families {
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
	for i := range families {
		fam := families[i]
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
		out.Message = noProvisionerMessage(provider, service)
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
	incomplete := false
	for _, m := range fm {
		if m.rank != best {
			continue
		}
		if len(crds) == maxProvisionerCRDs {
			warnings = append(warnings, fmt.Sprintf("more than %d %s CRDs for %s; only the first %d are listed", maxProvisionerCRDs, chosen.name, service, maxProvisionerCRDs))
			out.truncated, incomplete = true, true
			break
		}
		crd := &crdv1.CustomResourceDefinition{}
		if err := cli.Get(ctx, client.ObjectKey{Name: m.name}, crd); err != nil {
			warnings = append(warnings, fmt.Sprintf("CustomResourceDefinition %s could not be read: %s", m.name, redactLine(err.Error())))
			incomplete = true
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
		res, w, trimmed, err := provisionedResource(crd, chosen.health)
		if err != nil {
			return nil, warnings, err
		}
		out.Resource = res
		out.truncated = out.truncated || trimmed
		return out, append(warnings, w...), nil
	}
	kinds := make([]string, 0, len(out.Kinds))
	for _, k := range out.Kinds {
		kinds = append(kinds, k.Kind)
	}
	if incomplete {
		// A CRD that was not read may be the one asked for.
		out.Message = fmt.Sprintf("%s serves %s on this cluster, but not every %s CRD could be read, so whether it has a %s kind is unknown; the kinds read are %s",
			chosen.display, service, chosen.display, kind, strings.Join(kinds, ", "))
		return out, warnings, nil
	}
	out.Message = fmt.Sprintf("%s serves %s on this cluster, but has no %s kind; the installed kinds are %s", chosen.display, service, kind, strings.Join(kinds, ", "))
	return out, warnings, nil
}

func noProvisionerMessage(provider, service string) string {
	families := providerFamilies[provider]
	var names, groups, installs []string
	for _, fam := range families {
		var gs []string
		for _, suf := range fam.suffixes {
			gs = append(gs, service+suf)
		}
		names = append(names, fam.display)
		groups = append(groups, fmt.Sprintf("%s (%s)", fam.display, strings.Join(gs, ", ")))
		installs = append(installs, formatInstall(fam.install, service))
	}
	return fmt.Sprintf("Neither %s provides %s on this cluster: no CRDs were found in the groups of %s. "+
		"Creating %s resources from KubeVela needs one of them: install %s, or %s. "+
		"Ask the platform team which provisioner this cluster should use; a definition for a provisioner that is not installed cannot be checked or applied.",
		strings.Join(names, " nor "), service, strings.Join(groups, " or "), service, installs[0], installs[1])
}

// provisionedResource extracts the storage version's spec schema. A schema too
// large to return whole loses its descriptions, then its nested fields, then
// top-level fields in name order, and reports that it was cut.
func provisionedResource(crd *crdv1.CustomResourceDefinition, health string) (*ProvisionedResource, []string, bool, error) {
	v := storageVersion(crd)
	res := &ProvisionedResource{
		ProvisionedKind: ProvisionedKind{Kind: crd.Spec.Names.Kind, APIVersion: crd.Spec.Group + "/" + v.Name, Scope: string(crd.Spec.Scope)},
		Required:        []string{},
		Spec:            map[string]any{},
		Health:          health,
	}
	if v.Schema == nil || v.Schema.OpenAPIV3Schema == nil {
		return res, []string{fmt.Sprintf("CustomResourceDefinition %s publishes no schema", crd.Name)}, false, nil
	}
	spec, ok := v.Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		return res, []string{fmt.Sprintf("CustomResourceDefinition %s has no spec in its schema", crd.Name)}, false, nil
	}
	res.Required = append(res.Required, spec.Required...)
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, nil, false, err
	}
	var warnings []string
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, nil, false, err
	}
	if len(raw) <= maxSchemaBytes {
		res.Spec = m
		return res, nil, false, nil
	}
	dropDescriptions(m)
	if b, _ := json.Marshal(m); len(b) <= maxSchemaBytes {
		res.Spec = m
		return res, []string{fmt.Sprintf("the %s spec schema is larger than %d KiB; field descriptions were dropped", res.Kind, maxSchemaBytes>>10)}, true, nil
	}
	m, dropped := topLevelOnly(m, maxSchemaBytes)
	warnings = append(warnings, fmt.Sprintf("the %s spec schema is larger than %d KiB; only top-level fields and their types are returned", res.Kind, maxSchemaBytes>>10))
	if dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("%d top-level fields of %s did not fit and are left out; required fields are kept", dropped, res.Kind))
	}
	res.Spec = m
	return res, warnings, true, nil
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

// topLevelOnly keeps the spec's top-level fields with their types, required
// ones first, adding fields in name order only while the result stays within
// limit bytes. It returns how many fields were left out.
func topLevelOnly(spec map[string]any, limit int) (map[string]any, int) {
	out := map[string]any{"type": spec["type"]}
	required := map[string]bool{}
	if req, ok := spec["required"].([]any); ok {
		out["required"] = req
		for _, r := range req {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}
	props, _ := spec["properties"].(map[string]any)
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if required[names[i]] != required[names[j]] {
			return required[names[i]]
		}
		return names[i] < names[j]
	})
	short := map[string]any{}
	out["properties"] = short
	dropped := 0
	for _, name := range names {
		pm, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		short[name] = map[string]any{"type": pm["type"]}
		if b, _ := json.Marshal(out); len(b) > limit && !required[name] {
			delete(short, name)
			dropped++
		}
	}
	return out, dropped
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
