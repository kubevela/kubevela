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

package cuetest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/config"
)

// The Secret a hook's vela/config call stores a Config in is deleted with
// the rest of what the hook created.
func TestHookConfigIsCleanedUp(t *testing.T) {
	cl := testCluster(t)
	s := loadOne(t, "testdata/sources/configs_test.cue")
	for i, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures, s.Cases[i].Name)
	}
	requireGone(t, cl, "vela-system", "registry")
}

// A Config whose template names its Secret is cleaned up by that name.
func TestHookConfigNamedByItsTemplateIsCleanedUp(t *testing.T) {
	cl := testCluster(t)
	ctx := context.Background()
	for _, name := range []string{"vela-system", "renamed-ns"} {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		require.NoError(t, client.IgnoreAlreadyExists(cl.Client.Create(ctx, ns)))
	}
	factory := config.NewConfigFactory(cl.Client)
	tmpl, err := factory.ParseTemplate(ctx, "", []byte(`
metadata: name: "renamed"
template: {
	output: metadata: name: "renamed-" + context.name
	parameter: url: string
}`))
	require.NoError(t, err)
	require.NoError(t, factory.CreateOrUpdateConfigTemplate(ctx, "vela-system", tmpl))
	t.Cleanup(func() { require.NoError(t, factory.DeleteTemplate(context.Background(), "vela-system", "renamed")) })

	dir := t.TempDir()
	write(t, dir, "renamed_test.cue", `import (
	"vela/test"
	"vela/config"
)

store: config.#CreateConfig & {$params: {
	name:      "cfg"
	namespace: "renamed-ns"
	template:  "renamed"
	config: url: "https://registry.example.com"
}} @before()

"the Config is there while the file runs": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "unused", data: {}}
}
`)
	step, err := os.ReadFile("testdata/steps/publish.cue")
	require.NoError(t, err)
	write(t, dir, "publish.cue", string(step))
	s := loadOne(t, filepath.Join(dir, "renamed_test.cue"))
	for i, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures, s.Cases[i].Name)
	}
	requireGone(t, cl, "renamed-ns", "renamed-cfg")
}

func requireGone(t *testing.T, cl *TestCluster, namespace, name string) {
	t.Helper()
	secret := &unstructured.Unstructured{}
	secret.SetAPIVersion("v1")
	secret.SetKind("Secret")
	err := cl.Client.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, secret)
	require.True(t, apierrors.IsNotFound(err), "Secret %s/%s: want not found, got %v", namespace, name, err)
}
