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

package componentdefinition

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func extending(namespace, name, extends string) *v1beta1.ComponentDefinition {
	return &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       v1beta1.ComponentDefinitionSpec{Extends: extends},
	}
}

// clientOver builds a client with the same index the manager registers, so the
// handler under test looks things up the way it does in a cluster.
func clientOver(defs ...*v1beta1.ComponentDefinition) client.Client {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&v1beta1.ComponentDefinition{}, extendsIndex, func(obj client.Object) []string {
			cd, ok := obj.(*v1beta1.ComponentDefinition)
			if !ok || cd.Spec.Extends == "" {
				return nil
			}
			name := cd.Spec.Extends
			if i := strings.LastIndex(name, "@"); i > 0 {
				name = name[:i]
			}
			return []string{name}
		})
	for _, d := range defs {
		b = b.WithObjects(d)
	}
	return b.Build()
}

func namesOf(cli client.Client, changed *v1beta1.ComponentDefinition) []string {
	reqs := descendantsOf(cli)(context.Background(), changed)
	names := make([]string, 0, len(reqs))
	for _, r := range reqs {
		names = append(names, r.Name)
	}
	return names
}

// A change to a root has to reach the whole chain below it, not just its direct
// children. A middle level whose own spec did not change emits no update of its
// own, so anything under it would keep a schema describing a parent that moved.
func TestEveryDescendantWakes(t *testing.T) {
	cli := clientOver(
		extending("vela-system", "base", ""),
		extending("vela-system", "mid", "base"),
		extending("vela-system", "leaf", "mid"),
		extending("vela-system", "pinned-leaf", "mid@v2"),
		extending("vela-system", "unrelated", "something-else"),
	)

	names := namesOf(cli, extending("vela-system", "base", ""))

	require.ElementsMatch(t, []string{"mid", "leaf", "pinned-leaf"}, names)
}

// A chain that loops must not loop the walk.
func TestACycleDoesNotHangTheWalk(t *testing.T) {
	cli := clientOver(
		extending("vela-system", "a", "b"),
		extending("vela-system", "b", "a"),
	)

	// Only b: a is the definition that changed, and its own reconcile is already
	// under way. Reaching it again is what would loop.
	require.ElementsMatch(t, []string{"b"}, namesOf(cli, extending("vela-system", "a", "b")))
}

// Definitions are confined to one namespace, so the walk stays in it.
func TestTheWalkStaysInItsNamespace(t *testing.T) {
	cli := clientOver(
		extending("team-a", "base", ""),
		extending("team-a", "mine", "base"),
		extending("team-b", "theirs", "base"),
	)

	require.ElementsMatch(t, []string{"mine"}, namesOf(cli, extending("team-a", "base", "")))
}
