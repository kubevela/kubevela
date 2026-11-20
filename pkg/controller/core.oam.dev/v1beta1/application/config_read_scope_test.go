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

package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	cuexvelaconfig "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
	velaconfigreader "github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig/reader"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/registry"
)

// A definition that reads a Config through vela/velaconfig reads for the Application it renders,
// so it keeps to that Application's namespace, as a source does.
var _ = Describe("Reading a Config from a component definition", func() {
	It("refuses a Config in another namespace", func() {
		ctx := context.Background()
		registry.RegisterAs[cuexvelaconfig.Reader](velaconfigreader.Config{Client: k8sClient})
		ns := "cfg-read-scope"
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).Should(Succeed())
		def := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "reads-a-config", Namespace: ns},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "ConfigMap"}},
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `
import "vela/velaconfig"

read: velaconfig.#Read & {$params: {namespace: parameter.namespace, name: "db"}}
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	data: url: read.$returns.properties.url
}
parameter: namespace: string
`}},
			},
		}
		Expect(k8sClient.Create(ctx, def)).Should(Succeed())
		app := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "reads-elsewhere", Namespace: ns},
			Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{
				Name: "c", Type: "reads-a-config", Properties: &runtime.RawExtension{Raw: []byte(`{"namespace":"team-b"}`)},
			}}},
		}
		Expect(k8sClient.Create(ctx, app)).Should(Succeed())
		key := client.ObjectKeyFromObject(app)
		testutil.ReconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: key})

		Eventually(func(g Gomega) {
			testutil.ReconcileRetry(reconciler, reconcile.Request{NamespacedName: key})
			got := &v1beta1.Application{}
			g.Expect(k8sClient.Get(ctx, key, got)).Should(Succeed())
			g.Expect(statusMessages(got)).Should(ContainSubstring("config team-b/db is outside the application's namespace " + ns))
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())
	})
})

// statusMessages joins every message an Application's status carries.
func statusMessages(app *v1beta1.Application) string {
	var msgs []string
	for _, c := range app.Status.Conditions {
		msgs = append(msgs, fmt.Sprintf("%s: %s", c.Type, c.Message))
	}
	if app.Status.Workflow != nil {
		msgs = append(msgs, app.Status.Workflow.Message)
		for _, step := range app.Status.Workflow.Steps {
			msgs = append(msgs, fmt.Sprintf("%s: %s %s", step.Name, step.Reason, step.Message))
		}
	}
	return strings.Join(msgs, "\n")
}
