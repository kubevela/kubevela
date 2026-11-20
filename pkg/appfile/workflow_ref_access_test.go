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

package appfile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// workflowReaders answers access reviews: only the users named may get Workflows, in any
// namespace. It counts the reviews it answers.
type workflowReaders struct {
	client.Client
	readers map[string]bool
	asked   *int
}

func (w workflowReaders) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if sar, ok := obj.(*authv1.SubjectAccessReview); ok {
		*w.asked++
		ra := sar.Spec.ResourceAttributes
		sar.Status.Allowed = ra.Resource == "workflows" && ra.Verb == "get" && w.readers[sar.Spec.User]
		return nil
	}
	return w.Client.Create(ctx, obj, opts...)
}

// A shared Workflow is read with the controller's identity, so with AuthenticateApplication on
// its author must be able to read it themselves.
func TestAWorkflowRefIsReadAsTheAuthor(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	require.NoError(t, wfTypesv1alpha1.AddToScheme(scheme))
	shared := &wfTypesv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "release", Namespace: oam.SystemDefinitionNamespace},
		Steps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "wait", Type: "suspend"}}}}
	app := func(user string) *v1beta1.Application {
		return &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "team-a", Annotations: map[string]string{oam.AnnotationApplicationUsername: user}},
			Spec:       v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{}, Workflow: &v1beta1.Workflow{Ref: "release"}},
		}
	}
	parse := func(user string) (int, error) {
		asked := 0
		cli := workflowReaders{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(shared.DeepCopy()).Build(),
			readers: map[string]bool{"platform-dev": true}, asked: &asked}
		_, err := NewApplicationParser(cli).GenerateAppFileFromApp(context.Background(), app(user))
		return asked, err
	}

	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, true)
	// The author's username is trusted only with authentication.withUser on; otherwise every
	// Application runs as the default user.
	withUser := auth.AuthenticationWithUser
	auth.AuthenticationWithUser = true
	t.Cleanup(func() { auth.AuthenticationWithUser = withUser })
	_, err := parse("tenant")
	require.ErrorContains(t, err, `cannot read Workflow "vela-system/release"`)
	require.ErrorContains(t, err, `"tenant"`)
	asked, err := parse("platform-dev")
	require.NoError(t, err)
	require.Equal(t, 1, asked, "one review per parse, however often the ref is read")

	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, false)
	asked, err = parse("tenant")
	require.NoError(t, err, "without authentication there is no author to read as: shared as before")
	require.Zero(t, asked)
}
