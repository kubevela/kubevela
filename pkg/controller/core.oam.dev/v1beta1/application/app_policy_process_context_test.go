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

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestPolicyProcessContextPublishVersion(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
		Name: "shop", Namespace: "team-a",
		Annotations: map[string]string{oam.AnnotationPublishVersion: "v7"},
	}}
	pCtx := newPolicyProcessContext(context.Background(), app, "shop-v3", "audit", "audit", nil)
	file, err := pCtx.BaseContextFile()
	require.NoError(t, err)

	publishVersion, err := cuecontext.New().CompileString(file).LookupPath(cue.ParsePath("context.publishVersion")).String()
	require.NoError(t, err, "a policy template reads context.publishVersion as its components do")
	require.Equal(t, "v7", publishVersion)
}
