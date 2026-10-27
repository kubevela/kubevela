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

package addon

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// AffinityTraitType is the TraitDefinition used for pod scheduling overlays
	// (nodeSelector, affinity, tolerations, ...). It patches the primary workload
	// and, via patchOutputs, helmchart chart resources. On type: module (and
	// inline modules whose primary output is an Application) it also overlays
	// auxiliary controller Deployments inside the owned Application. Trait
	// fields take precedence over chart values already present on those resources.
	AffinityTraitType = "affinity"

	// TopologySpreadTraitType is attached when topologySpreadConstraints are set.
	TopologySpreadTraitType = "topologyspreadconstraints"

	// SchedulingArg is the reserved addon-args key that carries scheduling properties.
	// It is peeled before CUE render and re-injected into the args Secret for upgrade merge.
	SchedulingArg = "addonScheduling"

	// DeprecatedSchedulingTraitType is the POC trait name; rewritten to affinity on attach.
	DeprecatedSchedulingTraitType = "scheduling"
)

// SchedulingProperties holds standard Kubernetes PodSpec scheduling fields
// (CLI --scheduling-file / reserved args). They are mapped onto the affinity
// trait (and topologyspreadconstraints when needed).
type SchedulingProperties struct {
	NodeSelector              map[string]string      `json:"nodeSelector,omitempty"`
	Affinity                  map[string]interface{} `json:"affinity,omitempty"`
	Tolerations               []interface{}          `json:"tolerations,omitempty"`
	TopologySpreadConstraints []interface{}          `json:"topologySpreadConstraints,omitempty"`
	SchedulerName             string                 `json:"schedulerName,omitempty"`
	PriorityClassName         string                 `json:"priorityClassName,omitempty"`
}

// Empty reports whether no scheduling field is set.
func (s SchedulingProperties) Empty() bool {
	return len(s.NodeSelector) == 0 &&
		len(s.Affinity) == 0 &&
		len(s.Tolerations) == 0 &&
		len(s.TopologySpreadConstraints) == 0 &&
		s.SchedulerName == "" &&
		s.PriorityClassName == ""
}

// ToAffinityTraitProperties maps file/CLI fields onto affinity trait parameters.
func (s SchedulingProperties) ToAffinityTraitProperties() map[string]interface{} {
	m := map[string]interface{}{}
	if len(s.NodeSelector) > 0 {
		m["nodeSelector"] = s.NodeSelector
	}
	if len(s.Affinity) > 0 {
		m["affinity"] = s.Affinity
	}
	if len(s.Tolerations) > 0 {
		m["tolerations"] = s.Tolerations
	}
	if s.SchedulerName != "" {
		m["schedulerName"] = s.SchedulerName
	}
	if s.PriorityClassName != "" {
		m["priorityClassName"] = s.PriorityClassName
	}
	return m
}

// MergeNodeSelector overlays CLI key=value pairs onto scheduling props (CLI wins).
func (s *SchedulingProperties) MergeNodeSelector(ns map[string]string) {
	if len(ns) == 0 {
		return
	}
	if s.NodeSelector == nil {
		s.NodeSelector = map[string]string{}
	}
	for k, v := range ns {
		s.NodeSelector[k] = v
	}
}

// LoadSchedulingFile reads a YAML file of standard Kubernetes PodSpec scheduling fields.
func LoadSchedulingFile(path string) (SchedulingProperties, error) {
	var props SchedulingProperties
	if strings.TrimSpace(path) == "" {
		return props, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return props, errors.Wrapf(err, "read scheduling file %s", path)
	}
	if err := yaml.Unmarshal(data, &props); err != nil {
		return props, errors.Wrapf(err, "parse scheduling file %s", path)
	}
	return props, nil
}

// ExtractSchedulingFromArgs peels SchedulingArg from the addon args map.
func ExtractSchedulingFromArgs(args map[string]interface{}) (SchedulingProperties, error) {
	var props SchedulingProperties
	if args == nil {
		return props, nil
	}
	raw, ok := args[SchedulingArg]
	if !ok || raw == nil {
		return props, nil
	}
	delete(args, SchedulingArg)
	b, err := json.Marshal(raw)
	if err != nil {
		return props, errors.Wrap(err, "marshal scheduling args")
	}
	if err := json.Unmarshal(b, &props); err != nil {
		return props, errors.Wrap(err, "parse scheduling args")
	}
	return props, nil
}

// InjectSchedulingIntoArgs stores scheduling props under the reserved args key for Secret persistence.
func InjectSchedulingIntoArgs(args map[string]interface{}, props SchedulingProperties) {
	if args == nil || props.Empty() {
		return
	}
	b, err := json.Marshal(props)
	if err != nil {
		return
	}
	var asMap map[string]interface{}
	if err := json.Unmarshal(b, &asMap); err != nil {
		return
	}
	args[SchedulingArg] = asMap
}

// AttachSchedulingTrait appends or replaces affinity (and topology-spread when
// needed) on workload components, including type: module and helmchart.
// Namespace helper components are skipped. Affinity trait fields overlay chart
// values and module auxiliary controller pod templates.
func AttachSchedulingTrait(app *v1beta1.Application, props SchedulingProperties) error {
	if app == nil || props.Empty() {
		return nil
	}
	affinityProps := props.ToAffinityTraitProperties()
	var affinityRaw *runtime.RawExtension
	if len(affinityProps) > 0 {
		affinityRaw = util.Object2RawExtension(affinityProps)
		if affinityRaw == nil {
			return fmt.Errorf("failed to encode affinity trait properties")
		}
	}
	var topoRaw *runtime.RawExtension
	if len(props.TopologySpreadConstraints) > 0 {
		topoRaw = util.Object2RawExtension(map[string]interface{}{
			"constraints": props.TopologySpreadConstraints,
		})
		if topoRaw == nil {
			return fmt.Errorf("failed to encode topology spread trait properties")
		}
	}
	for i := range app.Spec.Components {
		comp := &app.Spec.Components[i]
		if shouldSkipSchedulingComponent(*comp) {
			continue
		}
		if affinityRaw != nil {
			upsertTrait(comp, AffinityTraitType, affinityRaw)
			removeTrait(comp, DeprecatedSchedulingTraitType)
		}
		if topoRaw != nil {
			upsertTrait(comp, TopologySpreadTraitType, topoRaw)
		}
	}
	return nil
}

func upsertTrait(comp *common2.ApplicationComponent, typ string, props *runtime.RawExtension) {
	for j := range comp.Traits {
		if comp.Traits[j].Type == typ {
			comp.Traits[j].Properties = props
			return
		}
	}
	comp.Traits = append(comp.Traits, common2.ApplicationTrait{
		Type:       typ,
		Properties: props,
	})
}

func removeTrait(comp *common2.ApplicationComponent, typ string) {
	out := comp.Traits[:0]
	for _, t := range comp.Traits {
		if t.Type == typ {
			continue
		}
		out = append(out, t)
	}
	comp.Traits = out
}

func shouldSkipSchedulingComponent(comp common2.ApplicationComponent) bool {
	if comp.Type == "raw" {
		return true
	}
	// Addon helpers are usually named *-ns (cert-manager-ns) or *-namespace.
	return strings.HasSuffix(comp.Name, "-ns") || strings.HasSuffix(comp.Name, "-namespace")
}
