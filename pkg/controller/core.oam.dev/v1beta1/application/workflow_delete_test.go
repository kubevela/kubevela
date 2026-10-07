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
	"context"
	"testing"

	workflowkube "github.com/kubevela/workflow/pkg/providers/kube"
	providertypes "github.com/kubevela/workflow/pkg/providers/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appkeeper"
	"github.com/oam-dev/kubevela/pkg/oam"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

// A workflow step that runs kube.#Delete reaches the cluster through the workflow's kube provider
// and then AppHandler.Delete, with the resource written in the step: its identity and nothing else.
// This runs that path end to end, so what protects a resource (here its sharer list) must be read
// from the object in the cluster and not from the step.
func TestWorkflowKubeDeleteRespectsSharedResource(t *testing.T) {
	for name, tc := range map[string]struct {
		annotations map[string]string
		deleted     bool
		sharedBy    string
	}{
		"a resource shared with another application is only unshared": {
			annotations: map[string]string{oam.AnnotationAppSharedBy: "default/app,other-ns/other-app"},
			sharedBy:    "other-ns/other-app",
		},
		"a resource nobody shares is deleted": {
			deleted: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			ctx := context.Background()
			cli := fake.NewClientBuilder().WithScheme(common2.Scheme).Build()
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Generation: 1}}
			keeper, err := appkeeper.New(ctx, cli, app)
			r.NoError(err)
			h := &AppHandler{Client: cli, app: app, resourceKeeper: keeper}

			r.NoError(cli.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name: "target", Namespace: "default", Annotations: tc.annotations,
				Labels: map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"},
			}}))

			// What the step carries: the resource's identity, nothing else.
			step := &unstructured.Unstructured{}
			step.SetAPIVersion("v1")
			step.SetKind("ConfigMap")
			step.SetName("target")
			step.SetNamespace("default")

			ret, err := workflowkube.Delete(ctx, &workflowkube.ResourceParams{
				Params: workflowkube.ResourceVars{Resource: step},
				RuntimeParams: providertypes.RuntimeParams{
					KubeHandlers: &providertypes.KubeHandlers{Delete: h.Delete},
					KubeClient:   cli,
				},
			})
			r.NoError(err)
			if ret != nil {
				r.Empty(ret.Returns.Error)
			}

			got := &corev1.ConfigMap{}
			err = cli.Get(ctx, types.NamespacedName{Namespace: "default", Name: "target"}, got)
			if tc.deleted {
				r.True(kerrors.IsNotFound(err))
				return
			}
			r.NoError(err, "a resource shared with another application must survive")
			r.Equal(tc.sharedBy, got.Annotations[oam.AnnotationAppSharedBy])
		})
	}
}
