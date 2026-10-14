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

package mcp

import (
	"context"
	"encoding/json"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const webserviceTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: template: spec: containers: [{
		image: parameter.image
		ports: [{containerPort: parameter.port}]
	}]
}
parameter: {
	// +usage=Which image would you like to use
	image: string
	// +usage=Which port to expose
	port: *80 | int
}
`

// harness runs a server over in-memory transports against a fake client.
type harness struct {
	ctx     context.Context
	cancel  context.CancelFunc
	session *sdk.ClientSession
	srv     *Server
}

// startHarness builds a server for opts over the given objects and connects
// a client to it. ServerVersion defaults to a fixed value so the envelope
// is deterministic.
func startHarness(opts Options, objs ...client.Object) *harness {
	h := &harness{}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(objs...).Build()
	f := velacmd.NewTestFactory(&rest.Config{Host: "https://example.invalid"}, cli)
	if opts.ServerVersion == nil {
		opts.ServerVersion = func(*rest.Config) (string, error) { return "v1.31.4", nil }
	}
	var err error
	h.srv, err = NewServer(f, opts)
	Expect(err).NotTo(HaveOccurred())

	clientTransport, serverTransport := sdk.NewInMemoryTransports()
	go func() { _ = h.srv.Serve(h.ctx, serverTransport) }()
	c := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "0"}, nil)
	h.session, err = c.Connect(h.ctx, clientTransport, nil)
	Expect(err).NotTo(HaveOccurred())
	return h
}

// call invokes a tool and decodes its structured content into `into` when
// the call succeeded.
func (h *harness) call(name string, args map[string]any, into any) *sdk.CallToolResult {
	res, err := h.session.CallTool(h.ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	Expect(err).NotTo(HaveOccurred())
	if into != nil && !res.IsError {
		raw, err := json.Marshal(res.StructuredContent)
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(raw, into)).To(Succeed())
	}
	return res
}

// errorText returns the text of a failed call for assertions.
func errorText(res *sdk.CallToolResult) string {
	Expect(res.IsError).To(BeTrue())
	Expect(res.Content).NotTo(BeEmpty())
	tc, ok := res.Content[0].(*sdk.TextContent)
	Expect(ok).To(BeTrue())
	return tc.Text
}

// toJSON renders a value for substring assertions on whole payloads.
func toJSON(v any) string {
	raw, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return string(raw)
}

func (h *harness) close() {
	if h.session != nil {
		_ = h.session.Close()
	}
	h.cancel()
}

// platformFixtures is a healthy vela-core Deployment plus a few definitions.
func platformFixtures() []client.Object {
	return []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kubevela-vela-core",
				Namespace: types.DefaultKubeVelaNS,
				Labels: map[string]string{
					controllerNameLabel: controllerNameValue,
					chartVersionLabel:   "1.11.0",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr.To[int32](1),
				Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "kubevela",
					Image: "oamdev/vela-core:v1.11.0",
					Args: []string{
						"--feature-gates=AuthenticateApplication=false",
						"--feature-gates=RestrictDefinitionNamespaces=true",
						"--feature-gates=EnableCueValidation=false,ApplyOnce=true",
					},
				}}}},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
		&v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "webservice",
				Namespace:   types.DefaultKubeVelaNS,
				Annotations: map[string]string{types.AnnoDefinitionDescription: "Describes long-running, scalable, containerized services"},
			},
			Spec: v1beta1.ComponentDefinitionSpec{
				Version:   "2.1.0",
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: webserviceTemplate}},
			},
			Status: v1beta1.ComponentDefinitionStatus{LatestRevision: &oamcommon.Revision{Name: "webservice-v3", Revision: 3}},
		},
		&v1beta1.TraitDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "scaler",
				Namespace:   types.DefaultKubeVelaNS,
				Annotations: map[string]string{types.AnnoDefinitionDescription: "Manually scale the workload"},
			},
			Spec: v1beta1.TraitDefinitionSpec{
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "patch: spec: replicas: parameter.replicas\nparameter: replicas: *1 | int\n"}},
			},
		},
		&v1beta1.PolicyDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "team-policy", Namespace: "team-a"},
			Spec: v1beta1.PolicyDefinitionSpec{
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: "parameter: {}\n"}},
			},
		},
	}
}
