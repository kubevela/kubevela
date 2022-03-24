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

package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emicklei/go-restful/v3"
	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
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
	})

	It("Test resource action", func() {
		ra := &RequestResourceAction{}
		ra.SetResourceWithName("project:{projectName}/workflow:{empty}", &testParam{})
		Expect(ra.GetResource()).ShouldNot(BeNil())
		Expect(ra.GetResource().Value).Should(BeEquivalentTo("projectName"))
		Expect(ra.GetResource().Next).ShouldNot(BeNil())
		Expect(ra.GetResource().Next.Value).Should(BeEquivalentTo("*"))
	})

	It("Test checkPerm by admin user", func() {
		err := ds.Add(context.TODO(), &model.Role{Name: "admin-role", PermPolicies: []string{"admin"}})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.PermPolicy{Name: "admin", Resources: []string{"*"}, Actions: []string{"*"}})
		Expect(err).Should(BeNil())

		rbac := rbacUsecase{ds: ds}
		req := &http.Request{}
		req = req.WithContext(context.WithValue(req.Context(), &apisv1.CtxKeyUser, &model.User{Name: "admin", UserRoles: []string{"admin-role"}}))
		res := &restful.Response{}
		pass := false
		filter := &restful.FilterChain{
			Target: restful.RouteFunction(func(req *restful.Request, res *restful.Response) {
				pass = true
			}),
		}
		rbac.CheckPerm("cluster", "create")(restful.NewRequest(req), res, filter)
		Expect(pass).Should(BeTrue())
	})

	It("Test checkPerm by dev user", func() {
		err := ds.Add(context.TODO(), &model.Role{Name: "application-admin", PermPolicies: []string{"application-manage"}})
		Expect(err).Should(BeNil())

		err = ds.Add(context.TODO(), &model.PermPolicy{Name: "application-manage", Resources: []string{"project:*/application:*"}, Actions: []string{"*"}})
		Expect(err).Should(BeNil())

		rbac := rbacUsecase{ds: ds}
		header := http.Header{}
		header.Set("Accept", "application/json")
		header.Set("Content-Type", "application/json")
		req := &http.Request{
			Header: header,
		}
		req = req.WithContext(context.WithValue(req.Context(), &apisv1.CtxKeyUser, &model.User{Name: "dev", UserRoles: []string{"application-admin"}}))
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

		rbac.CheckPerm("application", "list")(restful.NewRequest(req), res, filter)
		Expect(pass).Should(BeTrue())

	})
})

type testParam struct {
}

func (t *testParam) PathParameter(name string) string {
	if name == "empty" {
		return ""
	}
	return name
}
func TestRequestResourceAction(t *testing.T) {
	ra := &RequestResourceAction{}
	ra.SetResourceWithName("project:{projectName}/workflow:{empty}", &testParam{})
	assert.NotEqual(t, ra.GetResource(), nil)
	assert.Equal(t, ra.GetResource().Value, "projectName")
	assert.NotEqual(t, ra.GetResource().Next, nil)
	assert.Equal(t, ra.GetResource().Next.Value, "*")
}

func TestRequestResourceActionMatch(t *testing.T) {
	ra := &RequestResourceAction{}
	ra.SetResourceWithName("project:{projectName}/workflow:{empty}", &testParam{})
	ra.SetActions([]string{"create"})
	assert.Equal(t, ra.Match([]*model.PermPolicy{{Resources: []string{"project:*/workflow:*"}, Actions: []string{"*"}}}), true)
	assert.Equal(t, ra.Match([]*model.PermPolicy{{Resources: []string{"project:ddd/workflow:*"}, Actions: []string{"create"}}}), false)
	assert.Equal(t, ra.Match([]*model.PermPolicy{{Resources: []string{"project:projectName/workflow:*"}, Actions: []string{"create"}}}), true)
	assert.Equal(t, ra.Match([]*model.PermPolicy{{Resources: []string{"project:projectName/workflow:*"}, Actions: []string{"create"}, Effect: "Deny"}}), false)

	ra2 := &RequestResourceAction{}
	ra2.SetResourceWithName("project:{projectName}/application:{app1}/component:{empty}", &testParam{})
	ra2.SetActions([]string{"delete"})
	assert.Equal(t, ra2.Match([]*model.PermPolicy{{Resources: []string{"project:*/application:app1/component:*"}, Actions: []string{"*"}}}), true)
	assert.Equal(t, ra2.Match([]*model.PermPolicy{{Resources: []string{"project:*/application:app1/component:*"}, Actions: []string{"list", "delete"}}}), true)
	assert.Equal(t, ra2.Match([]*model.PermPolicy{{Resources: []string{"project:*", "project:*/application:app1/component:*"}, Actions: []string{"list", "delete"}}}), true)
	assert.Equal(t, ra2.Match([]*model.PermPolicy{{Resources: []string{"project:*/application:app1/component:*"}, Actions: []string{"list", "detail"}}}), false)
	assert.Equal(t, ra2.Match([]*model.PermPolicy{{Resources: []string{"*"}, Actions: []string{"*"}}}), true)
}
