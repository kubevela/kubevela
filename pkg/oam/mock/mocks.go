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

// Package mock provides fake OAM resources for use in tests.
package mock

import (
	"encoding/json"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
)

// Conditioned is a mock that implements Conditioned interface.
type Conditioned struct{ Conditions []condition.Condition }

// SetConditions sets the Conditions.
func (m *Conditioned) SetConditions(c ...condition.Condition) { m.Conditions = c }

// GetCondition get the Condition with the given ConditionType.
func (m *Conditioned) GetCondition(ct condition.ConditionType) condition.Condition {
	return condition.Condition{Type: ct, Status: corev1.ConditionUnknown}
}

// ManagedResourceReferencer is a mock that implements ManagedResourceReferencer interface.
type ManagedResourceReferencer struct {
	Ref *corev1.ObjectReference `json:"ref"`
}

// SetResourceReference sets the ResourceReference.
func (m *ManagedResourceReferencer) SetResourceReference(r *corev1.ObjectReference) { m.Ref = r }

// GetResourceReference gets the ResourceReference.
func (m *ManagedResourceReferencer) GetResourceReference() *corev1.ObjectReference { return m.Ref }

// A WorkloadReferencer references an OAM Workload type.
type WorkloadReferencer struct {
	Ref corev1.ObjectReference `json:"ref"`
}

// GetWorkloadReference gets the WorkloadReference.
func (w *WorkloadReferencer) GetWorkloadReference() corev1.ObjectReference {
	return w.Ref
}

// SetWorkloadReference sets the WorkloadReference.
func (w *WorkloadReferencer) SetWorkloadReference(r corev1.ObjectReference) {
	w.Ref = r
}

// Object is a mock that implements Object interface.
type Object struct {
	metav1.ObjectMeta
	runtime.Object
}

// GetObjectKind returns schema.ObjectKind.
func (o *Object) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (o *Object) DeepCopyObject() runtime.Object {
	out := &Object{}
	j, err := json.Marshal(o)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}

// Trait is a mock that implements Trait interface.
type Trait struct {
	metav1.ObjectMeta
	runtime.Object
	condition.ConditionedStatus
	WorkloadReferencer
}

// GetObjectKind returns schema.ObjectKind.
func (t *Trait) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (t *Trait) DeepCopyObject() runtime.Object {
	out := &Trait{}
	j, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}

// Workload is a mock that implements Workload interface.
type Workload struct {
	metav1.ObjectMeta
	runtime.Object
	condition.ConditionedStatus
}

// GetObjectKind returns schema.ObjectKind.
func (w *Workload) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// DeepCopyObject returns a copy of the object as runtime.Object
func (w *Workload) DeepCopyObject() runtime.Object {
	out := &Workload{}
	j, err := json.Marshal(w)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}

// Manager is a mock object that satisfies manager.Manager interface.
type Manager struct {
	manager.Manager

	Client client.Client
	Scheme *runtime.Scheme
}

// GetClient returns the client.
func (m *Manager) GetClient() client.Client { return m.Client }

// GetScheme returns the scheme.
func (m *Manager) GetScheme() *runtime.Scheme { return m.Scheme }

// GetConfig returns the config for test.
func (m *Manager) GetConfig() *rest.Config {
	return &rest.Config{}
}

// GV returns a mock schema.GroupVersion.
var GV = schema.GroupVersion{Group: "g", Version: "v"}

// GVK returns the mock GVK of the given object.
func GVK(o runtime.Object) schema.GroupVersionKind {
	return GV.WithKind(reflect.TypeOf(o).Elem().Name())
}

// SchemeWith returns a scheme with list of `runtime.Object`s registered.
func SchemeWith(o ...runtime.Object) *runtime.Scheme {
	s := runtime.NewScheme()
	s.AddKnownTypes(GV, o...)
	return s
}

// NotFoundErr describes NotFound Resource error
type NotFoundErr struct {
	NotFoundStatus metav1.Status
}

// NewMockNotFoundErr return a mock NotFoundErr
func NewMockNotFoundErr() NotFoundErr {
	return NotFoundErr{
		NotFoundStatus: metav1.Status{
			Reason: metav1.StatusReasonNotFound,
		},
	}
}

// Status returns the Status Reason
func (mock NotFoundErr) Status() metav1.Status {
	return mock.NotFoundStatus
}

// Error return error info
func (mock NotFoundErr) Error() string {
	return "Not Found Resource"
}

// A LocalSecretReference is a reference to a secret in the same namespace as
// the referencer.
type LocalSecretReference struct {
	// Name of the secret.
	Name string `json:"name"`
}

// LocalConnectionSecretWriterTo is a mock that implements LocalConnectionSecretWriterTo interface.
type LocalConnectionSecretWriterTo struct {
	Ref *LocalSecretReference `json:"local_secret_ref"`
}

// SetWriteConnectionSecretToReference sets the WriteConnectionSecretToReference.
func (m *LocalConnectionSecretWriterTo) SetWriteConnectionSecretToReference(r *LocalSecretReference) {
	m.Ref = r
}

// GetWriteConnectionSecretToReference gets the WriteConnectionSecretToReference.
func (m *LocalConnectionSecretWriterTo) GetWriteConnectionSecretToReference() *LocalSecretReference {
	return m.Ref
}

// Target is a mock that implements Target interface.
type Target struct {
	metav1.ObjectMeta
	ManagedResourceReferencer
	LocalConnectionSecretWriterTo
	condition.ConditionedStatus
}

// GetObjectKind returns schema.ObjectKind.
func (m *Target) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

// DeepCopyObject returns a deep copy of Target as runtime.Object.
func (m *Target) DeepCopyObject() runtime.Object {
	out := &Target{}
	j, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	_ = json.Unmarshal(j, out)
	return out
}
