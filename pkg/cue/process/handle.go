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

package process

import (
	"context"
	"strconv"
	"strings"

	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ContextData is the core data of process context
type ContextData struct {
	Namespace       string
	Cluster         string
	AppName         string
	CompName        string
	StepName        string
	AppRevisionName string
	WorkflowName    string
	PublishVersion  string
	ReplicaKey      string

	Ctx            context.Context
	BaseHooks      []process.BaseHook
	AuxiliaryHooks []process.AuxiliaryHook
	Components     []common.ApplicationComponent

	AppLabels      map[string]string
	AppAnnotations map[string]string

	ClusterVersion types.ClusterVersion
}

// NewContext creates a new process context
func NewContext(data ContextData) process.Context {
	ctx := process.NewContext(process.ContextData{
		Namespace:      data.Namespace,
		Name:           data.CompName,
		StepName:       data.StepName,
		WorkflowName:   data.WorkflowName,
		PublishVersion: data.PublishVersion,
		Ctx:            data.Ctx,
		BaseHooks:      data.BaseHooks,
		AuxiliaryHooks: data.AuxiliaryHooks,
	})
	ctx.PushData(ContextAppName, data.AppName)
	ctx.PushData(ContextAppRevision, data.AppRevisionName)
	ctx.PushData(ContextCompRevisionName, ComponentRevisionPlaceHolder)
	ctx.PushData(ContextComponents, data.Components)
	ctx.PushData(ContextAppLabels, data.AppLabels)
	ctx.PushData(ContextAppAnnotations, data.AppAnnotations)
	ctx.PushData(ContextReplicaKey, data.ReplicaKey)
	revNum, _ := util.ExtractRevisionNum(data.AppRevisionName, "-")
	ctx.PushData(ContextAppRevisionNum, revNum)
	ctx.PushData(ContextCluster, data.Cluster)
	ctx.PushData(ContextClusterVersion, parseClusterVersion(data.ClusterVersion))
	return ctx
}

func parseClusterVersion(cv types.ClusterVersion) map[string]interface{} {
	// no minor found, use control plane cluster version instead.
	if cv.Minor == "" {
		cv = types.ControlPlaneClusterVersion
	}
	minorS := strings.TrimSpace(cv.Minor)
	minorS = strings.TrimRight(minorS, ".+-/?!")
	minor, _ := strconv.ParseInt(minorS, 10, 64)
	return map[string]interface{}{
		"major":      cv.Major,
		"gitVersion": cv.GitVersion,
		"platform":   cv.Platform,
		"minor":      minor,
	}
}
