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

// Package componenthealth evaluates a component's health from its live resources without
// an Application or a render context: everything rendering would have supplied is passed
// in, as the hub puts it in a Component for vela-agent (design/vela-core/agent-dispatch.md).
package componenthealth

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	outputField    = "output"
	outputsField   = "outputs"
	parameterField = "parameter"
)

// Spec is everything needed to evaluate one component's health where it runs.
type Spec struct {
	// Context holds the render-time context values health CUE reads as context.*
	// (see definition.GetBaseContextLabels: name, appName, appRevision).
	Context map[string]string
	// Workload is nil when the workload is not applied (it is managed by a trait).
	Workload *Subject
	Traits   []Trait
}

// Subject is a workload or trait whose status is evaluated.
type Subject struct {
	// Status carries the health, customStatus and details CUE, and the parameters.
	Status health.StatusRequest
	// Output is the workload's main resource (context.output). Traits have none.
	Output *ResourceRef
	// Outputs are the auxiliary resources (context.outputs.<name>).
	Outputs map[string]ResourceRef
}

// Trait is a trait's subject, with its type.
type Trait struct {
	Type string
	Subject
}

// ResourceRef identifies a rendered resource by its final name.
type ResourceRef struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

// Evaluate fetches each subject's live resources and evaluates its status. Evaluation is
// best-effort, as in the Application controller: a CUE error is logged and the partial
// result used. A resource that cannot be fetched is an error.
//
// The Application in common.ApplicationComponentStatus and ApplicationTraitStatus is
// historical: both describe a component, and neither holds anything Application-specific.
func Evaluate(ctx context.Context, cli client.Reader, spec Spec) (*common.ApplicationComponentStatus, error) {
	status := &common.ApplicationComponentStatus{Healthy: true, Details: make(map[string]string)}
	if spec.Workload != nil {
		result, err := evaluate(ctx, cli, spec.Context, *spec.Workload)
		if err != nil {
			return nil, err
		}
		status.Healthy = result.Healthy
		if result.Message != "" {
			status.Message = result.Message
		}
		if result.Details != nil {
			status.Details = result.Details
		}
		status.WorkloadHealthy = status.Healthy
	}
	traits := make([]common.ApplicationTraitStatus, 0, len(spec.Traits))
	for _, trait := range spec.Traits {
		result, err := evaluate(ctx, cli, spec.Context, trait.Subject)
		if err != nil {
			return nil, errors.WithMessagef(err, "trait %s", trait.Type)
		}
		if status.Message == "" && result.Message != "" {
			status.Message = result.Message
		}
		traits = append(traits, common.ApplicationTraitStatus{
			Type: trait.Type, Healthy: result.Healthy, Message: result.Message, Details: result.Details,
		})
	}
	status.Traits = traits
	Rollup(status, spec.Workload != nil)
	return status, nil
}

func evaluate(ctx context.Context, cli client.Reader, baseContext map[string]string, subject Subject) (*health.StatusResult, error) {
	templateContext := make(map[string]interface{}, len(baseContext)+3)
	for k, v := range baseContext {
		templateContext[k] = v
	}
	if subject.Output != nil {
		obj, err := get(ctx, cli, *subject.Output)
		if err != nil {
			return nil, err
		}
		templateContext[outputField] = obj.Object
	}
	if len(subject.Outputs) > 0 {
		outputs := make(map[string]interface{}, len(subject.Outputs))
		for name, ref := range subject.Outputs {
			obj, err := get(ctx, cli, ref)
			if err != nil {
				return nil, err
			}
			outputs[name] = obj.Object
		}
		templateContext[outputsField] = outputs
	}
	templateContext[parameterField] = subject.Status.Parameter
	request := subject.Status
	result, err := health.GetStatus(templateContext, &request)
	if err != nil {
		klog.Warningf("evaluate status error (best-effort): %v", err)
	}
	return result, nil
}

func get(ctx context.Context, cli client.Reader, ref ResourceRef) (*unstructured.Unstructured, error) {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid apiVersion %q for %s %s/%s", ref.APIVersion, ref.Kind, ref.Namespace, ref.Name)
	}
	obj, err := util.GetObjectGivenGVKAndName(ctx, cli, gv.WithKind(ref.Kind), ref.Namespace, ref.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get %s %s/%s", ref.Kind, ref.Namespace, ref.Name)
	}
	return obj, nil
}

// Rollup applies the component health rule to status, once its workload and traits have been
// evaluated: healthy when the workload is (if there is one) and no trait that has finished is
// unhealthy. With no workload, unhealthy traits get a default message.
func Rollup(status *common.ApplicationComponentStatus, hasWorkload bool) {
	skipWorkload, traits := !hasWorkload, status.Traits
	traitHealthy := true
	for _, ts := range traits {
		if ts.Pending {
			continue
		}
		if !ts.Healthy {
			traitHealthy = false
			break
		}
	}
	if !skipWorkload {
		status.Healthy = status.WorkloadHealthy && traitHealthy
	} else if !traitHealthy {
		status.Healthy = false
		if status.Message == "" {
			status.Message = "traits are not healthy"
		}
	}
}
