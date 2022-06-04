/*
Copyright 2022 The KubeVela Authors.

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

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/emicklei/go-restful/v3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
)

var _ = Describe("Test rbac service", func() {
	var ds datastore.DataStore
	BeforeEach(func() {
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "rbac-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
	})
	It("Test check resource", func() {
		path, err := checkResourcePath("project")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}"))

		path, err = checkResourcePath("application")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/application:{appName}"))

		path, err = checkResourcePath("environment")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/environment:{envName}"))

		_, err = checkResourcePath("applications")
		Expect(err).ShouldNot(BeNil())

		_, err = checkResourcePath("project/component")
		Expect(err).ShouldNot(BeNil())

		_, err = checkResourcePath("workflow")
		Expect(err).ShouldNot(BeNil())

		path, err = checkResourcePath("project/application/workflow")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/application:{appName}/workflow:{workflowName}"))

		path, err = checkResourcePath("project/workflow")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/workflow:{workflowName}"))

		path, err = checkResourcePath("component")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("project:{projectName}/application:{appName}/component:{compName}"))

		path, err = checkResourcePath("role")
		Expect(err).Should(BeNil())
		Expect(path).Should(BeEquivalentTo("role:*"))

	})

	It("Test resource action", func() {
		ra := &RequestResourceAction{}
		ra.SetResourceWithName("project:{projectName}/workflow:{empty}", testPathParameter)
		Expect(ra.GetResource()).ShouldNot(BeNil())
		Expect(ra.GetResource().Value).Should(BeEquivalentTo("projectName"))
		Expect(ra.GetResource().Next).ShouldNot(BeNil())
		Expect(ra.GetResource().Next.Value).Should(BeEquivalentTo("*"))
		Expect(ra.GetResource().String()).Should(BeEquivalentTo("project:projectName/workflow:*"))
	})

	It("Test init and list platform permissions", func() {
		rbacService := rbacServiceImpl{Store: ds}
		err := rbacService.Init(context.TODO())
		Expect(err).Should(BeNil())
		policies, err := rbacService.ListPermissions(context.TODO(), "")
		Expect(err).Should(BeNil())
		Expect(len(policies)).Should(BeEquivalentTo(int64(8)))
	})

	It("Test checkPerm by admin user", func() {

		err := ds.Add(context.TODO(), &model.User{Name: "admin", UserRoles: []string{"admin"}})
		Expect(err).Should(BeNil())

		rbac := rbacServiceImpl{Store: ds}
		req := &http.Request{}
		req = req.WithContext(context.WithValue(req.Context(), &apisv1.CtxKeyUser, "admin"))
		res := &restful.Response{}
		pass := false
		filter := &restful.FilterChain{
			Target: restful.RouteFunction(func(req *restful.Request, res *restful.Response) {
				pass = true
			}),
		}
		rbac.CheckPerm("cluster", "create")(restful.NewRequest(req), res, filter)
		Expect(pass).Should(BeTrue())
		pass = false
		rbac.CheckPerm("role", "list")(restful.NewRequest(req), res, filter)
		Expect(pass).Should(BeTrue())
	})

	It("Test checkPerm by dev user", func() {
		var projectName = "test-app-project"

		err := ds.Add(context.TODO(), &model.User{Name: "dev"})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.Project{Name: projectName})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.ProjectUser{Username: "dev", ProjectName: projectName, UserRoles: []string{"application-admin"}})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.Role{Project: projectName, Name: "application-admin", Permissions: []string{"application-manage"}})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.Permission{Project: projectName, Name: "application-manage", Resources: []string{"project:test-app-project/application:*"}, Actions: []string{"*"}})
		Expect(err).Should(BeNil())

		rbac := rbacServiceImpl{Store: ds}
		header := http.Header{}
		header.Set("Accept", "application/json")
		header.Set("Content-Type", "application/json")
		req := &http.Request{
			Header: header,
		}
		req = req.WithContext(context.WithValue(req.Context(), &apisv1.CtxKeyUser, "dev"))
		req.Form = url.Values{}
		req.Form.Set("project", projectName)

		record := httptest.NewRecorder()
		res := restful.NewResponse(record)
		res.SetRequestAccepts("application/json")
		pass := false
		filter := &restful.FilterChain{
			Target: restful.RouteFunction(func(req *restful.Request, res *restful.Response) {
				pass = true
			}),
		}
		rbac.CheckPerm("cluster", "create")(restful.NewRequest(req), res, filter)
		Expect(pass).Should(BeFalse())
		Expect(res.StatusCode()).Should(Equal(int(bcode.ErrForbidden.HTTPCode)))

		rbac.CheckPerm("component", "list")(restful.NewRequest(req), res, filter)
		Expect(res.StatusCode()).Should(Equal(int(bcode.ErrForbidden.HTTPCode)))

		// add list application permission to role
		// err = Store.Add(context.TODO(), &model.Permission{Project: projectName, Name: "application-list", Resources: []string{"project:*/application:*"}, Actions: []string{"list"}})
		// Expect(err).Should(BeNil())
		// _, err = rbac.UpdateRole(context.TODO(), projectName, "application-admin", apisv1.UpdateRoleRequest{
		// 	Permissions: []string{"application-list", "application-manage"},
		// })
		// Expect(err).Should(BeNil())

		// req.Form.Del("project")
		// pass = false
		// rbac.CheckPerm("application", "list")(restful.NewRequest(req), res, filter)
		// Expect(pass).Should(BeTrue())
	})

	It("Test initDefaultRoleAndUsersForProject", func() {
		rbacService := rbacServiceImpl{Store: ds}
		err := ds.Add(context.TODO(), &model.User{Name: "test-user"})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.Project{Name: "init-test", Owner: "test-user"})
		Expect(err).Should(BeNil())
		err = rbacService.InitDefaultRoleAndUsersForProject(context.TODO(), &model.Project{Name: "init-test"})
		Expect(err).Should(BeNil())

		roles, err := rbacService.ListRole(context.TODO(), "init-test", 0, 0)
		Expect(err).Should(BeNil())
		Expect(roles.Total).Should(BeEquivalentTo(int64(2)))

		policies, err := rbacService.ListPermissions(context.TODO(), "init-test")
		Expect(err).Should(BeNil())
		Expect(len(policies)).Should(BeEquivalentTo(int64(5)))
	})

	It("Test UpdatePermission", func() {
		rbacService := rbacServiceImpl{Store: ds}
		base, err := rbacService.UpdatePermission(context.TODO(), "test-app-project", "application-manage", &apisv1.UpdatePermissionRequest{
			Resources: []string{"project:{projectName}/application:*/*"},
			Actions:   []string{"*"},
			Alias:     "App Management Update",
		})
		Expect(err).Should(BeNil())
		Expect(base.Alias).Should(BeEquivalentTo("App Management Update"))
	})
})

func testPathParameter(name string) string {
	if name == "empty" {
		return ""
	}
	return name
}
func TestRequestResourceAction(t *testing.T) {
	ra := &RequestResourceAction{}
	ra.SetResourceWithName("project:{projectName}/workflow:{empty}", testPathParameter)
	assert.NotEqual(t, ra.GetResource(), nil)
	assert.Equal(t, ra.GetResource().Value, "projectName")
	assert.NotEqual(t, ra.GetResource().Next, nil)
	assert.Equal(t, ra.GetResource().Next.Value, "*")

	ra2 := &RequestResourceAction{}
	ra2.SetResourceWithName("project:{empty}/application:{empty}", testPathParameter)
	assert.Equal(t, ra2.GetResource().String(), "project:*/application:*")
}

func TestRequestResourceActionMatch(t *testing.T) {
	ra := &RequestResourceAction{}
	ra.SetResourceWithName("project:{projectName}/workflow:{empty}", testPathParameter)
	ra.SetActions([]string{"create"})
	assert.Equal(t, ra.Match([]*model.Permission{{Resources: []string{"project:*/workflow:*"}, Actions: []string{"*"}}}), true)
	assert.Equal(t, ra.Match([]*model.Permission{{Resources: []string{"project:ddd/workflow:*"}, Actions: []string{"create"}}}), false)
	assert.Equal(t, ra.Match([]*model.Permission{{Resources: []string{"project:projectName/workflow:*"}, Actions: []string{"create"}}}), true)
	assert.Equal(t, ra.Match([]*model.Permission{{Resources: []string{"project:projectName/workflow:*"}, Actions: []string{"create"}, Effect: "Deny"}}), false)

	ra2 := &RequestResourceAction{}
	ra2.SetResourceWithName("project:{projectName}/application:{app1}/component:{empty}", testPathParameter)
	ra2.SetActions([]string{"delete"})
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"project:*/application:app1/component:*"}, Actions: []string{"*"}}}), true)
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"project:*/application:app1/component:*"}, Actions: []string{"list", "delete"}}}), true)
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"project:*", "project:*/application:app1/component:*"}, Actions: []string{"list", "delete"}}}), true)
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"project:*/application:app1/component:*"}, Actions: []string{"list", "detail"}}}), false)
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"*"}, Actions: []string{"*"}}}), true)
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"*"}, Actions: []string{"*"}}, {Actions: []string{"*"}, Resources: []string{"project:*/application:app1/component:*"}, Effect: "Deny"}}), false)
	assert.Equal(t, ra2.Match([]*model.Permission{{Resources: []string{"project:projectName/application:*/*"}, Actions: []string{"*"}}}), true)

	ra3 := &RequestResourceAction{}
	ra3.SetResourceWithName("project:test-123", testPathParameter)
	ra3.SetActions([]string{"detail"})
	assert.Equal(t, ra3.Match([]*model.Permission{{Resources: []string{"*"}, Actions: []string{"*"}, Effect: "Allow"}}), true)

	ra4 := &RequestResourceAction{}
	ra4.SetResourceWithName("role:*", testPathParameter)
	ra4.SetActions([]string{"list"})
	assert.Equal(t, ra4.Match([]*model.Permission{{Resources: []string{"*"}, Actions: []string{"*"}, Effect: "Allow"}}), true)

	ra5 := &RequestResourceAction{}
	ra5.SetResourceWithName("project:*/application:*", testPathParameter)
	ra5.SetActions([]string{"list"})
	assert.Equal(t, ra5.Match([]*model.Permission{{Resources: []string{"project:*/application:*"}, Actions: []string{"list"}, Effect: "Allow"}}), true)

	ra6 := &RequestResourceAction{}
	path, err := checkResourcePath("environment")
	assert.Equal(t, err, nil)
	ra6.SetResourceWithName(path, func(name string) string {
		if name == "projectName" {
			return "default"
		}
		return ""
	})
	ra6.SetActions([]string{"create"})
	assert.Equal(t, ra6.Match([]*model.Permission{{Resources: []string{
		"project:*/*", "addon:* addonRegistry:*", "target:*", "cluster:*/namespace:*", "user:*", "role:*", "permission:*", "configType:*/*", "project:*",
		"project:default/config:*", "project:default/role:*", "project:default/projectUser:*", "project:default/permission:*", "project:default/environment:*", "project:default/application:*/*", "project:default",
	}, Actions: []string{"list", "detail"}, Effect: "Allow"}}), false)

}

func TestRegisterResourceAction(t *testing.T) {
	registerResourceAction("role", "list")
	registerResourceAction("project/role", "list")
	t.Log(resourceActions)
}
