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

package webservice

import (
	"fmt"
	"strings"
	"sync"

	"github.com/emicklei/go-restful/v3"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// resourceActions all register resources and actions
var resourceActions map[string][]string
var lock sync.Mutex

// ResourceMaps all resources definition for RBAC
var ResourceMaps = map[string]interface{}{
	"Project": map[string]interface{}{
		"Application": map[string]interface{}{
			"Component": map[string]interface{}{
				"Trait": map[string]interface{}{},
			},
			"Workflow": map[string]interface{}{
				"Record": map[string]interface{}{},
			},
			"Policy":     map[string]interface{}{},
			"Revision":   map[string]interface{}{},
			"EnvBinding": map[string]interface{}{},
			"Trigger":    map[string]interface{}{},
		},
		"Environment":         map[string]interface{}{},
		"Workflow":            map[string]interface{}{},
		"Role":                map[string]interface{}{},
		"ProjectUser":         map[string]interface{}{},
		"ApplicationTemplate": map[string]interface{}{},
	},
	"Cluster":       map[string]interface{}{},
	"Addon":         map[string]interface{}{},
	"Target":        map[string]interface{}{},
	"User":          map[string]interface{}{},
	"Role":          map[string]interface{}{},
	"SystemSetting": map[string]interface{}{},
	"Definition":    map[string]interface{}{},
}

var existResourcePaths = convertMap2Array(ResourceMaps)

func checkResourcePath(resource string) (string, error) {
	path := ""
	exist := 0
	for _, erp := range existResourcePaths {
		index := strings.Index(erp, fmt.Sprintf("/%s/", resource))
		if index > -1 {
			pre := erp[:index+len(resource)+2]
			if pre != path {
				exist++
			}
			path = pre
		}
	}
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimPrefix(path, "/")
	if exist == 1 {
		return path, nil
	}
	if exist > 1 {
		return path, fmt.Errorf("the resource name %s is not unique", resource)
	}
	return path, fmt.Errorf("there is no resource %s", resource)
}

func convertMap2Array(sources map[string]interface{}) (list []string) {
	for k, v := range sources {
		subResource := v.(map[string]interface{})
		if len(subResource) > 0 {
			for _, sub := range convertMap2Array(subResource) {
				list = append(list, fmt.Sprintf("/%s%s", k, sub))
			}
		}
		if len(subResource) == 0 {
			list = append(list, fmt.Sprintf("/%s/", k))
		}
	}
	return
}

type permWebService struct {
}

// NewPermWebService is the webservice of user perm
func NewPermWebService() WebService {
	return &permWebService{}
}

func (p *permWebService) GetWebService() *restful.WebService {
	ws := new(restful.WebService)
	return ws
}

// RegisterResourceAction register resource actions
func RegisterResourceAction(resource string, actions ...string) {
	lock.Lock()
	defer lock.Unlock()
	if resourceActions == nil {
		resourceActions = make(map[string][]string)
	}
	if _, exist := ResourceMaps[resource]; !exist {
		path, err := checkResourcePath(resource)
		if err != nil {
			panic(fmt.Sprintf("resource %s is not exist", resource))
		}
		resource = path
	}
	if _, exist := resourceActions[resource]; exist {
		for _, action := range actions {
			if !utils.StringsContain(resourceActions[resource], action) {
				resourceActions[resource] = append(resourceActions[resource], action)
			}
		}
	} else {
		resourceActions[resource] = actions
	}
}

func (p *permWebService) checkPermFilter(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	RegisterResourceAction(resource, actions...)
	f := func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
		// get user info from req.Request.Context()

		// get use's perm list.

		// match resource and resource name form req.PathParameter()
		// Application: req.PathParameter("name")
		// Component: req.PathParameter("compName")
		// EnvBinding: req.PathParameter("envName")

		// match perm policy
	}
	return f
}
