/*
Copyright 2019 The Crossplane Authors.

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

// Package oam contains miscellaneous OAM helper types.
package oam

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
)

// ScopeKind contains the type metadata for a kind of an OAM scope resource.
type ScopeKind schema.GroupVersionKind

// TraitKind contains the type metadata for a kind of an OAM trait resource.
type TraitKind schema.GroupVersionKind

// WorkloadKind contains the type metadata for a kind of an OAM workload resource.
type WorkloadKind schema.GroupVersionKind

// A Conditioned may have conditions set or retrieved. Conditions are typically
// indicate the status of both a resource and its reconciliation process.
type Conditioned interface {
	SetConditions(c ...runtimev1alpha1.Condition)
	GetCondition(runtimev1alpha1.ConditionType) runtimev1alpha1.Condition
}

// A WorkloadReferencer may reference an OAM workload.
type WorkloadReferencer interface {
	GetWorkloadReference() runtimev1alpha1.TypedReference
	SetWorkloadReference(runtimev1alpha1.TypedReference)
}

// A WorkloadsReferencer may reference an OAM workload.
type WorkloadsReferencer interface {
	GetWorkloadReferences() []runtimev1alpha1.TypedReference
	AddWorkloadReference(runtimev1alpha1.TypedReference)
}

// A Finalizer manages the finalizers on the resource.
type Finalizer interface {
	AddFinalizer(ctx context.Context, obj Object) error
	RemoveFinalizer(ctx context.Context, obj Object) error
}

// An Object is a Kubernetes object.
type Object interface {
	metav1.Object
	runtime.Object
}

// A Trait is a type of OAM trait.
type Trait interface {
	Object

	Conditioned
	WorkloadReferencer
}

// A Scope is a type of OAM scope.
type Scope interface {
	Object

	Conditioned
	WorkloadsReferencer
}

// A Workload is a type of OAM workload.
type Workload interface {
	Object

	Conditioned
}
