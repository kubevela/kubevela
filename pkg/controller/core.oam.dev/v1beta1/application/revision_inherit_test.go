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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func inheritedDef(name, extends, template string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1beta1.ComponentDefinitionSpec{
			Extends:   extends,
			Schematic: &common.Schematic{CUE: &common.CUE{Template: template}},
		},
	}
}

func revisionWith(defs map[string]*v1beta1.ComponentDefinition) *v1beta1.ApplicationRevision {
	return &v1beta1.ApplicationRevision{
		Spec: v1beta1.ApplicationRevisionSpec{
			ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
				Application:          v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "acme-billing"}},
				ComponentDefinitions: defs,
			},
		},
	}
}

// Recording ancestors is what lets an application notice its parent changing.
//
// The hash covers every definition in the map, so once a parent is in there,
// editing it produces a different hash. Without the ancestor recorded the hash
// would be identical and nothing downstream could tell the difference.
//
// The hash changing is necessary but not sufficient for a re-render, and it is
// worth being exact about which. Unless `app.oam.dev/autoUpdate` is set on the
// app, currentAppRevIsNew decides on deepEqualAppInRevision, which compares the
// policies, the workflow and the Application spec, and looks at no definition at
// all. So by default a definition edit does not roll a revision: definitions are
// pinned at first render, and tracking them is opted into. Verified on a
// cluster, where the same edit produced no new revision without the annotation
// and acme-billing-v2 with it. This test pins the hash, not the policy.
func TestEditingAParentChangesTheRevisionHash(t *testing.T) {
	child := inheritedDef("tenant-webservice", "webservice", "$super: properties: {image: parameter.image}")

	before, err := ComputeAppRevisionHash(revisionWith(map[string]*v1beta1.ComponentDefinition{
		"tenant-webservice": child,
		"webservice":        inheritedDef("webservice", "", "output: replicas: 1"),
	}))
	require.NoError(t, err)

	after, err := ComputeAppRevisionHash(revisionWith(map[string]*v1beta1.ComponentDefinition{
		"tenant-webservice": child,
		"webservice":        inheritedDef("webservice", "", "output: replicas: 2"),
	}))
	require.NoError(t, err)

	require.NotEqual(t, before, after, "a parent edit must roll the application revision")
}

// And the same chain, unchanged, must keep its hash, or every reconcile would
// make a new revision.
func TestAnUnchangedChainKeepsItsHash(t *testing.T) {
	defs := func() map[string]*v1beta1.ComponentDefinition {
		return map[string]*v1beta1.ComponentDefinition{
			"tenant-webservice": inheritedDef("tenant-webservice", "webservice", "$super: properties: {image: parameter.image}"),
			"webservice":        inheritedDef("webservice", "", "output: replicas: 1"),
		}
	}

	first, err := ComputeAppRevisionHash(revisionWith(defs()))
	require.NoError(t, err)
	second, err := ComputeAppRevisionHash(revisionWith(defs()))
	require.NoError(t, err)

	require.Equal(t, first, second)
}
