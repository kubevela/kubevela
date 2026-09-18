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

// Package requiredcrds verifies at startup that the CRDs vela-core depends on are
// served by the cluster.
//
// Distinct from cmd/core/app/hooks/crdvalidation, which asks whether a CRD's
// schema is new enough for the feature gates that are on. This asks whether the
// CRD is there at all, which has to be answered first: that hook round-trips an
// ApplicationRevision, and against a missing CRD it fails on a no-match error
// that says nothing about what is wrong.
//
// Helm applies a chart's crds/ directory on install and never again: not on
// upgrade, not with --force. It is a deliberate refusal on Helm's part, since
// deleting a CRD takes every custom resource with it.
//
// Where the chart also ships instances of that kind, as it does for the nine
// built-in SourceDefinitions, `helm upgrade` fails outright with "ensure CRDs
// are installed first" and nothing changes. That is the loud case and it needs
// no help from this package.
//
// The quiet cases are what this is for: a CRD deleted by hand, a partial
// `kubectl apply` of the manifests, a chart that adds a watched kind with no
// instances to trip Helm's check. Six kinds are watched with For(), and a watch
// on a kind the API server does not serve takes the manager down with it, so one
// missing CRD stops Application reconciliation entirely over a feature the
// operator may not even use. The rest fail later and further away, on some
// user's Application rather than in front of whoever is doing the upgrade.
//
// This changes neither outcome. It moves the failure to the front and gives it a
// cause: which CRDs are missing, and the two commands that fix it.
package requiredcrds

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// CRD is one custom resource definition vela-core expects to find.
type CRD struct {
	// Plural is the resource name, matching the CRD's spec.names.plural.
	Plural string
	// GroupVersionKind identifies the kind to look up.
	schema.GroupVersionKind
}

// Name is the CRD's own name, <plural>.<group>, which is what kubectl prints and
// what an operator will search for.
func (c CRD) Name() string { return c.Plural + "." + c.Group }

func core(plural, kind, version string) CRD {
	return CRD{
		Plural:           plural,
		GroupVersionKind: schema.GroupVersionKind{Group: "core.oam.dev", Version: version, Kind: kind},
	}
}

// Required are the CRDs vela-core cannot run without. Each is either watched by a
// controller or read on the reconcile path, so its absence is not a missing
// feature but a broken install.
var Required = []CRD{
	core("applications", "Application", "v1beta1"),
	core("applicationrevisions", "ApplicationRevision", "v1beta1"),
	core("resourcetrackers", "ResourceTracker", "v1beta1"),
	core("componentdefinitions", "ComponentDefinition", "v1beta1"),
	core("traitdefinitions", "TraitDefinition", "v1beta1"),
	core("policydefinitions", "PolicyDefinition", "v1beta1"),
	core("workflowstepdefinitions", "WorkflowStepDefinition", "v1beta1"),
	core("sourcedefinitions", "SourceDefinition", "v1beta1"),
	core("definitionrevisions", "DefinitionRevision", "v1beta1"),
	core("workloaddefinitions", "WorkloadDefinition", "v1beta1"),
	core("policies", "Policy", "v1alpha1"),
	core("workflows", "Workflow", "v1alpha1"),
}

// Optional are CRDs the chart ships whose absence vela-core survives. Listed
// rather than omitted so that adding a CRD to the chart forces a decision about
// it: TestEveryChartCRDIsClassified fails on anything in neither list.
var Optional = []CRD{
	// Both callers of LoadExternalPackages sit behind a flag and log-and-continue
	// on failure, so a missing CRD costs the cuex external package feature and
	// nothing else.
	{
		Plural:           "packages",
		GroupVersionKind: schema.GroupVersionKind{Group: "cue.oam.dev", Version: "v1alpha1", Kind: "Package"},
	},
}

// Verify reports every required CRD the cluster does not serve.
//
// A NoKindMatchError means the CRD is not installed. Any other error means
// discovery itself failed, which usually means the API server is not up yet, and
// reporting that as a missing CRD sends an operator down the wrong path.
func Verify(mapper meta.RESTMapper) error {
	var missing []CRD
	for _, c := range Required {
		if _, err := mapper.RESTMapping(c.GroupKind(), c.Version); err != nil {
			if meta.IsNoMatchError(err) {
				missing = append(missing, c)
				continue
			}
			return fmt.Errorf("cannot determine whether CRD %s is installed: %w", c.Name(), err)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s", message(missing))
}

// message lists every missing CRD, not just the first: fixing them one restart at
// a time is the difference between one upgrade and four.
func message(missing []CRD) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d required CustomResourceDefinition(s) are not installed on this cluster:\n\n",
		len(missing))
	for _, c := range missing {
		fmt.Fprintf(&b, "  %s\n", c.Name())
	}
	b.WriteString(`
Helm applies a chart's CRDs on first install only and never on upgrade, so a
` + "`helm upgrade`" + ` to a version that adds one leaves it missing. Install them from
the chart you are upgrading to:

  helm pull kubevela/vela-core --version <version> --untar
  kubectl apply -f vela-core/crds/

` + "`vela install`" + ` applies the chart's CRDs itself and needs no separate step.

https://kubevela.io/docs/platform-engineers/system-operation/migration-from-old-version`)
	return b.String()
}
