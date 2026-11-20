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

package configsecret

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/utils/ptr"

	"github.com/oam-dev/kubevela/apis/types"
)

func configSecret(annotations map[string]string, owned bool, value string) *corev1.Secret {
	s := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "team-b", Namespace: "shop",
			Labels:      map[string]string{types.LabelConfigCatalog: types.VelaCoreConfig, types.LabelConfigType: "team"},
			Annotations: annotations,
		},
		Data: map[string][]byte{"input-properties": []byte(value)},
	}
	if owned {
		s.OwnerReferences = []metav1.OwnerReference{{APIVersion: "config.oam.dev/v1alpha1", Kind: "Config", Name: "team-b", UID: "cfg-uid", Controller: ptr.To(true)}}
	}
	return s
}

func update(t *testing.T, oldObj, newObj *corev1.Secret) admission.Response {
	t.Helper()
	raw := func(o *corev1.Secret) runtime.RawExtension {
		b, err := json.Marshal(o)
		require.NoError(t, err)
		return runtime.RawExtension{Raw: b}
	}
	h := &ValidatingHandler{Decoder: admission.NewDecoder(runtime.NewScheme())}
	return h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Update,
		Resource:  metav1.GroupVersionResource{Version: "v1", Resource: "secrets"},
		Name:      "team-b", Namespace: "shop",
		Object: raw(newObj), OldObject: raw(oldObj),
	}})
}

func TestAnAdoptedConfigSecretKeepsItsMigrationMark(t *testing.T) {
	adopted := map[string]string{types.AnnotationConfigMigratedTo: "cfg-uid", "config.oam.dev/alias": ""}
	plain := map[string]string{"config.oam.dev/alias": ""}

	t.Run("an old client's rewrite, dropping the mark and the owner, is refused", func(t *testing.T) {
		resp := update(t, configSecret(adopted, true, `{"owner":"v1"}`), configSecret(plain, false, `{"owner":"v2"}`))
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "config shop/team-b is held by the Config shop/team-b")
	})
	t.Run("the Config controller's write, keeping the mark, is allowed", func(t *testing.T) {
		resp := update(t, configSecret(adopted, true, `{"owner":"v1"}`), configSecret(adopted, true, `{"owner":"v2"}`))
		require.True(t, resp.Allowed, resp.Result)
	})
	t.Run("the garbage collector orphaning it, dropping only the owner, is allowed", func(t *testing.T) {
		resp := update(t, configSecret(adopted, true, `{"owner":"v1"}`), configSecret(adopted, false, `{"owner":"v1"}`))
		require.True(t, resp.Allowed, resp.Result)
	})
	// The webhook's objectSelector matches an update whose old object carries the catalog label, so
	// dropping the label is seen here; refused, the mark cannot then be dropped out of its sight.
	t.Run("dropping the catalog label, which the webhook selects on, is refused", func(t *testing.T) {
		unlabelled := configSecret(adopted, true, `{"owner":"v1"}`)
		delete(unlabelled.Labels, types.LabelConfigCatalog)
		resp := update(t, configSecret(adopted, true, `{"owner":"v1"}`), unlabelled)
		require.False(t, resp.Allowed)
		require.Contains(t, resp.Result.Message, "config shop/team-b is held by the Config shop/team-b")
	})
	t.Run("a legacy config never migrated is written as before", func(t *testing.T) {
		resp := update(t, configSecret(plain, false, `{"owner":"v1"}`), configSecret(plain, false, `{"owner":"v2"}`))
		require.True(t, resp.Allowed, resp.Result)
	})
}
