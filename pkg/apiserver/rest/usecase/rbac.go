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
	"fmt"
	"strings"
	"sync"

	"github.com/emicklei/go-restful/v3"

	"github.com/oam-dev/kubevela/pkg/apiserver/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// resourceActions all register resources and actions
var resourceActions map[string][]string
var lock sync.Mutex

type resourceMetadata struct {
	subResources map[string]resourceMetadata
	pathName     string
}

// ResourceMaps all resources definition for RBAC
var ResourceMaps = map[string]resourceMetadata{
	"project": {
		subResources: map[string]resourceMetadata{
			"application": {
				pathName: "appName",
				subResources: map[string]resourceMetadata{
					"component": {
						subResources: map[string]resourceMetadata{
							"trait": {
								pathName: "traitType",
							},
						},
						pathName: "compName",
					},
					"workflow": {
						subResources: map[string]resourceMetadata{
							"record": {
								pathName: "record",
							},
						},
						pathName: "workflowName",
					},
					"policy": {
						pathName: "policyName",
					},
					"revision": {
						pathName: "revision",
					},
					"envBinding": {
						pathName: "envName",
					},
					"trigger": {},
				},
			},
			"environment": {
				pathName: "envName",
			},
			"workflow": {
				pathName: "workflowName",
			},
			"role":                {},
			"projectUser":         {},
			"applicationTemplate": {},
		},
		pathName: "projectName",
	},
	"cluster": {
		pathName: "clusterName",
	},
	"addon": {
		pathName: "addonName",
	},
	"addonRegistry": {
		pathName: "addonRegName",
	},
	"target": {
		pathName: "targetName",
	},
	"user":          {},
	"role":          {},
	"systemSetting": {},
	"definition":    {},
}

var existResourcePaths = convertMap2Array(ResourceMaps)

func checkResourcePath(resource string) (string, error) {
	path := ""
	exist := 0
	lastResourceName := resource[strings.LastIndex(resource, "/")+1:]
	for key, erp := range existResourcePaths {
		allMatchIndex := strings.Index(key, fmt.Sprintf("/%s/", resource))
		index := strings.Index(erp, fmt.Sprintf("/%s:", lastResourceName))
		if index > -1 && allMatchIndex > -1 {
			pre := erp[:index+len(lastResourceName)+2]
			next := strings.Replace(erp, pre, "", 1)
			nameIndex := strings.Index(next, "/")
			if nameIndex > -1 {
				pre += next[:nameIndex]
			}
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

func convertMap2Array(sources map[string]resourceMetadata) map[string]string {
	list := make(map[string]string)
	for k, v := range sources {
		if len(v.subResources) > 0 {
			for sub, subWithPathName := range convertMap2Array(v.subResources) {
				if subWithPathName != "" {
					withPathname := fmt.Sprintf("/%s:*%s", k, subWithPathName)
					if v.pathName != "" {
						withPathname = fmt.Sprintf("/%s:{%s}%s", k, v.pathName, subWithPathName)
					}
					list[fmt.Sprintf("/%s%s", k, sub)] = withPathname
				}
			}
		}
		withPathname := fmt.Sprintf("/%s:*/", k)
		if v.pathName != "" {
			withPathname = fmt.Sprintf("/%s:{%s}/", k, v.pathName)
		}
		list[fmt.Sprintf("/%s/", k)] = withPathname
	}
	return list
}

type rbacUsecase struct {
	ds datastore.DataStore
}

// RBACUsecase implement RBAC-related business logic.
type RBACUsecase interface {
	CheckPerm(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain)
}

// NewRBACUsecase is the usecase service of RBAC
func NewRBACUsecase(ds datastore.DataStore) RBACUsecase {
	return &rbacUsecase{
		ds: ds,
	}
}

// registerResourceAction register resource actions
func registerResourceAction(resource string, actions ...string) {
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

func (p *rbacUsecase) CheckPerm(resource string, actions ...string) func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
	registerResourceAction(resource, actions...)
	f := func(req *restful.Request, res *restful.Response, chain *restful.FilterChain) {
		// get user info from req.Request.Context()

		// get use's perm list.

		// match resource and resource name form req.PathParameter(), build resource permission requirements
		_, err := checkResourcePath(resource)
		if err != nil {
			log.Logger.Errorf("check resource path failure %s", err.Error())
			bcode.ReturnError(req, res, bcode.ErrForbidden)
			return
		}

		// match perm policy

		chain.ProcessFilter(req, res)
	}
	return f
}
