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

package cli

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestApplyConfigTemplateCRDRefusesASystemTemplateOutsideVelaSystem(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).Build()
	tmpl := &config.Template{
		NamespacedName: config.NamespacedName{Name: "t"},
		Scope:          "system",
		Template:       script.CUE("template: {parameter: {}}"),
	}

	err := applyConfigTemplateCRD(ctx, cli, "wf-a", tmpl)
	r.Error(err)
	r.Contains(err.Error(), "config template wf-a/t: scope system is only valid in vela-system")
	r.True(apierrors.IsNotFound(cli.Get(ctx, client.ObjectKey{Namespace: "wf-a", Name: "t"}, &configv1alpha1.ConfigTemplate{})), "nothing written")

	r.NoError(applyConfigTemplateCRD(ctx, cli, "vela-system", tmpl))
	var got configv1alpha1.ConfigTemplate
	r.NoError(cli.Get(ctx, client.ObjectKey{Namespace: "vela-system", Name: "t"}, &got))
	r.Equal(configv1alpha1.ConfigTemplateScopeSystem, got.Spec.Scope)
}
