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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TraitKind contains the type metadata for a kind of an OAM trait resource.
type TraitKind schema.GroupVersionKind

// WorkloadKind contains the type metadata for a kind of an OAM workload resource.
type WorkloadKind schema.GroupVersionKind

// A Conditioned may have conditions set or retrieved. Conditions are typically
// indicate the status of both a resource and its reconciliation process.
type Conditioned interface {
	SetConditions(c ...condition.Condition)
	GetCondition(condition.ConditionType) condition.Condition
}

// A WorkloadReferencer may reference an OAM workload.
type WorkloadReferencer interface {
	GetWorkloadReference() corev1.ObjectReference
	SetWorkloadReference(corev1.ObjectReference)
}

// A WorkloadsReferencer may reference an OAM workload.
type WorkloadsReferencer interface {
	GetWorkloadReferences() []corev1.ObjectReference
	AddWorkloadReference(corev1.ObjectReference)
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

// A Workload is a type of OAM workload.
type Workload interface {
	Object

	Conditioned
}
