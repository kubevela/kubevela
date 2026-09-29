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
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velacommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

func dependenciesFixture() *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"},
		Status: common.AppStatus{
			Dependencies: []common.ComponentDependency{
				{Component: "api", DependsOn: "cfg", Source: common.DependencySourceDependsOn},
				{Component: "api", DependsOn: "db", Source: common.DependencySourceExpression},
				{Component: "api", DependsOn: "flags", Source: common.DependencySourceExpression, Cluster: "east"},
				{Component: "api", DependsOn: "q", Source: common.DependencySourceExpression, Namespace: "infra"},
				{Component: "web", DependsOn: "auth", Source: common.DependencySourceInputs},
			},
			Services: []common.ApplicationComponentStatus{
				{Name: "api", Cluster: "west", Healthy: false, Message: `waiting for component "flags" in east to be healthy`},
				{Name: "api", Cluster: "east", Healthy: true},
				{Name: "db", Cluster: "local", Healthy: false, Message: "not ready"},
			},
		},
	}
}

func TestDependencyWhere(t *testing.T) {
	r := require.New(t)
	deps := dependenciesFixture().Status.Dependencies
	r.Equal("-", dependencyWhere(deps[0]))
	r.Equal("beside", dependencyWhere(deps[1]))
	r.Equal("east", dependencyWhere(deps[2]))
	r.Equal("namespace infra", dependencyWhere(deps[3]), "not to be mistaken for a cluster called infra")
	r.Equal("local/ops", dependencyWhere(common.ComponentDependency{Source: common.DependencySourceExpression, Cluster: "local", Namespace: "ops"}))
}

func TestPrintAppDependencies(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(dependenciesFixture()).Build()

	var buf bytes.Buffer
	r.NoError(printAppDependencies(cli, "default", "shop", Filter{}, "", &buf))
	out := buf.String()
	r.Contains(out, "Dependencies of default/shop:")
	r.Regexp(`api\s+\|\s+cfg\s+\|\s+dependsOn\s+\|\s+-`, out)
	r.Regexp(`api\s+\|\s+db\s+\|\s+expression\s+\|\s+beside`, out)
	r.Regexp(`api\s+\|\s+flags\s+\|\s+expression\s+\|\s+east`, out)
	r.Regexp(`web\s+\|\s+auth\s+\|\s+inputs\s+\|\s+-`, out)
	r.Contains(out, `api (west): waiting for component "flags" in east to be healthy`,
		"a component still waiting says why")
	r.NotContains(out, "not ready", "only dependents' waits are shown")

	buf.Reset()
	r.NoError(printAppDependencies(cli, "default", "shop", Filter{Component: "web"}, "", &buf))
	r.NotContains(buf.String(), "flags", "--component narrows to one component")

	buf.Reset()
	r.NoError(printAppDependencies(cli, "default", "shop", Filter{}, "json", &buf))
	var got dependenciesOutput
	r.NoError(json.Unmarshal(buf.Bytes(), &got))
	r.Equal("shop", got.Name)
	r.Len(got.Dependencies, 5)
}

func TestPrintAppDependenciesNone(t *testing.T) {
	r := require.New(t)
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "plain", Namespace: "default"}}
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(app).Build()
	var buf bytes.Buffer
	r.NoError(printAppDependencies(cli, "default", "plain", Filter{}, "", &buf))
	r.Equal("Application default/plain has no component dependencies.\n", buf.String())
}

// A component filter that matches nothing says so, rather than implying the
// Application has no dependencies at all.
func TestPrintAppDependenciesUnmatchedComponent(t *testing.T) {
	r := require.New(t)
	cli := fake.NewClientBuilder().WithScheme(velacommon.Scheme).WithObjects(dependenciesFixture()).Build()
	var buf bytes.Buffer
	r.NoError(printAppDependencies(cli, "default", "shop", Filter{Component: "db"}, "", &buf))
	r.Equal("Component db of default/shop has no dependencies.\n", buf.String())
}
