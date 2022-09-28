/*
Copyright 2021 The KubeVela Authors.

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

package common

import (
	"context"
	"strings"

	plur "github.com/gertd/go-pluralize"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// ListRawWorkloadDefinitions will list raw definition
func ListRawWorkloadDefinitions(userNamespace string, c common.Args) ([]v1beta1.WorkloadDefinition, error) {
	client, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	ctx := util.SetNamespaceInCtx(context.Background(), userNamespace)
	workloadList := v1beta1.WorkloadDefinitionList{}
	ns := ctx.Value(util.AppDefinitionNamespace).(string)
	if err = client.List(ctx, &workloadList, client2.InNamespace(ns)); err != nil {
		return nil, err
	}
	if ns == oam.SystemDefinitionNamespace {
		return workloadList.Items, nil
	}
	sysWorkloadList := v1beta1.WorkloadDefinitionList{}
	if err = client.List(ctx, &sysWorkloadList, client2.InNamespace(oam.SystemDefinitionNamespace)); err != nil {
		return nil, err
	}
	return append(workloadList.Items, sysWorkloadList.Items...), nil
}

// ConvertApplyTo will convert applyTo slice to workload capability name if CRD matches
func ConvertApplyTo(applyTo []string, workloads []types.Capability) []string {
	var converted []string
	if in(applyTo, "*") {
		converted = append(converted, "*")
	} else {
		for _, v := range applyTo {
			newName, exist := check(v, workloads)
			if !exist {
				continue
			}
			if !in(converted, newName) {
				converted = append(converted, newName)
			}
		}
	}
	return converted
}

func check(applyto string, workloads []types.Capability) (string, bool) {
	for _, v := range workloads {
		if Parse(applyto) == v.CrdName || Parse(applyto) == v.Name {
			return v.Name, true
		}
	}
	return "", false
}

func in(l []string, v string) bool {
	for _, ll := range l {
		if ll == v {
			return true
		}
	}
	return false
}

// Parse will parse applyTo(with format Group/Version.Kind) to crd name by just calculate the plural of kind word.
// TODO we should use discoverymapper instead of calculate plural
func Parse(applyTo string) string {
	l := strings.Split(applyTo, "/")
	if len(l) != 2 {
		return applyTo
	}
	apigroup, versionKind := l[0], l[1]
	l = strings.Split(versionKind, ".")
	if len(l) != 2 {
		return applyTo
	}
	return plur.NewClient().Plural(strings.ToLower(l[1])) + "." + apigroup
}
