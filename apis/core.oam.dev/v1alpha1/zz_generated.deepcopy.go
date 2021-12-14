// +build !ignore_autogenerated

/*
Copyright 2021 The KubeVela Authors.

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ApplyOncePolicySpec) DeepCopyInto(out *ApplyOncePolicySpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ApplyOncePolicySpec.
func (in *ApplyOncePolicySpec) DeepCopy() *ApplyOncePolicySpec {
	if in == nil {
		return nil
	}
	out := new(ApplyOncePolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterConnection) DeepCopyInto(out *ClusterConnection) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterConnection.
func (in *ClusterConnection) DeepCopy() *ClusterConnection {
	if in == nil {
		return nil
	}
	out := new(ClusterConnection)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvBindingSpec) DeepCopyInto(out *EnvBindingSpec) {
	*out = *in
	if in.Envs != nil {
		in, out := &in.Envs, &out.Envs
		*out = make([]EnvConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvBindingSpec.
func (in *EnvBindingSpec) DeepCopy() *EnvBindingSpec {
	if in == nil {
		return nil
	}
	out := new(EnvBindingSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvBindingStatus) DeepCopyInto(out *EnvBindingStatus) {
	*out = *in
	if in.Envs != nil {
		in, out := &in.Envs, &out.Envs
		*out = make([]EnvStatus, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.ClusterConnections != nil {
		in, out := &in.ClusterConnections, &out.ClusterConnections
		*out = make([]ClusterConnection, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvBindingStatus.
func (in *EnvBindingStatus) DeepCopy() *EnvBindingStatus {
	if in == nil {
		return nil
	}
	out := new(EnvBindingStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvComponentPatch) DeepCopyInto(out *EnvComponentPatch) {
	*out = *in
	if in.Properties != nil {
		in, out := &in.Properties, &out.Properties
		*out = new(runtime.RawExtension)
		(*in).DeepCopyInto(*out)
	}
	if in.Traits != nil {
		in, out := &in.Traits, &out.Traits
		*out = make([]EnvTraitPatch, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvComponentPatch.
func (in *EnvComponentPatch) DeepCopy() *EnvComponentPatch {
	if in == nil {
		return nil
	}
	out := new(EnvComponentPatch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvConfig) DeepCopyInto(out *EnvConfig) {
	*out = *in
	in.Placement.DeepCopyInto(&out.Placement)
	if in.Selector != nil {
		in, out := &in.Selector, &out.Selector
		*out = new(EnvSelector)
		(*in).DeepCopyInto(*out)
	}
	in.Patch.DeepCopyInto(&out.Patch)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvConfig.
func (in *EnvConfig) DeepCopy() *EnvConfig {
	if in == nil {
		return nil
	}
	out := new(EnvConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvPatch) DeepCopyInto(out *EnvPatch) {
	*out = *in
	if in.Components != nil {
		in, out := &in.Components, &out.Components
		*out = make([]EnvComponentPatch, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvPatch.
func (in *EnvPatch) DeepCopy() *EnvPatch {
	if in == nil {
		return nil
	}
	out := new(EnvPatch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvPlacement) DeepCopyInto(out *EnvPlacement) {
	*out = *in
	if in.ClusterSelector != nil {
		in, out := &in.ClusterSelector, &out.ClusterSelector
		*out = new(common.ClusterSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.NamespaceSelector != nil {
		in, out := &in.NamespaceSelector, &out.NamespaceSelector
		*out = new(NamespaceSelector)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvPlacement.
func (in *EnvPlacement) DeepCopy() *EnvPlacement {
	if in == nil {
		return nil
	}
	out := new(EnvPlacement)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvSelector) DeepCopyInto(out *EnvSelector) {
	*out = *in
	if in.Components != nil {
		in, out := &in.Components, &out.Components
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvSelector.
func (in *EnvSelector) DeepCopy() *EnvSelector {
	if in == nil {
		return nil
	}
	out := new(EnvSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvStatus) DeepCopyInto(out *EnvStatus) {
	*out = *in
	if in.Placements != nil {
		in, out := &in.Placements, &out.Placements
		*out = make([]PlacementDecision, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvStatus.
func (in *EnvStatus) DeepCopy() *EnvStatus {
	if in == nil {
		return nil
	}
	out := new(EnvStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *EnvTraitPatch) DeepCopyInto(out *EnvTraitPatch) {
	*out = *in
	if in.Properties != nil {
		in, out := &in.Properties, &out.Properties
		*out = new(runtime.RawExtension)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new EnvTraitPatch.
func (in *EnvTraitPatch) DeepCopy() *EnvTraitPatch {
	if in == nil {
		return nil
	}
	out := new(EnvTraitPatch)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GarbageCollectPolicyRule) DeepCopyInto(out *GarbageCollectPolicyRule) {
	*out = *in
	in.Selector.DeepCopyInto(&out.Selector)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GarbageCollectPolicyRule.
func (in *GarbageCollectPolicyRule) DeepCopy() *GarbageCollectPolicyRule {
	if in == nil {
		return nil
	}
	out := new(GarbageCollectPolicyRule)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GarbageCollectPolicyRuleSelector) DeepCopyInto(out *GarbageCollectPolicyRuleSelector) {
	*out = *in
	if in.TraitTypes != nil {
		in, out := &in.TraitTypes, &out.TraitTypes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GarbageCollectPolicyRuleSelector.
func (in *GarbageCollectPolicyRuleSelector) DeepCopy() *GarbageCollectPolicyRuleSelector {
	if in == nil {
		return nil
	}
	out := new(GarbageCollectPolicyRuleSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GarbageCollectPolicySpec) DeepCopyInto(out *GarbageCollectPolicySpec) {
	*out = *in
	if in.Rules != nil {
		in, out := &in.Rules, &out.Rules
		*out = make([]GarbageCollectPolicyRule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GarbageCollectPolicySpec.
func (in *GarbageCollectPolicySpec) DeepCopy() *GarbageCollectPolicySpec {
	if in == nil {
		return nil
	}
	out := new(GarbageCollectPolicySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NamespaceSelector) DeepCopyInto(out *NamespaceSelector) {
	*out = *in
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NamespaceSelector.
func (in *NamespaceSelector) DeepCopy() *NamespaceSelector {
	if in == nil {
		return nil
	}
	out := new(NamespaceSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PlacementDecision) DeepCopyInto(out *PlacementDecision) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PlacementDecision.
func (in *PlacementDecision) DeepCopy() *PlacementDecision {
	if in == nil {
		return nil
	}
	out := new(PlacementDecision)
	in.DeepCopyInto(out)
	return out
}
