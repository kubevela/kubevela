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

package application

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/appfile"
)

func TestTheResolvedWorkflowRefIsReported(t *testing.T) {
	require.Nil(t, resolvedWorkflowRef(&appfile.Appfile{}), "no ref, nothing to report")
	af := &appfile.Appfile{ExternalWorkflow: &wfTypesv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "release", Namespace: "vela-system", UID: "u-1", ResourceVersion: "7"},
	}}
	require.Equal(t, &corev1.ObjectReference{APIVersion: "core.oam.dev/v1alpha1", Kind: "Workflow", Namespace: "vela-system", Name: "release"},
		resolvedWorkflowRef(af), "names the Workflow (a typed read carries no kind), without the version it was read at")
}
