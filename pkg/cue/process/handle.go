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
	"time"

	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// ContextData is the core data of process context
type ContextData struct {
	Namespace       string
	Cluster         string
	AppName         string
	CompName        string
	StepName        string
	CompRevision    string
	AppRevisionName string
	WorkflowName    string
	PublishVersion  string
	ReplicaKey      string

	Ctx             context.Context
	BaseHooks       []process.BaseHook
	AuxiliaryHooks  []process.AuxiliaryHook
	Components      []common.ApplicationComponent
	Sources         map[string]map[string]interface{}
	SourceTypes     map[string]string
	SourceTemplates map[string]string
	// SourceSensitivePaths maps source definition type to non-overridable sensitive field paths.
	SourceSensitivePaths map[string][]string
	// SourceCacheStore is used for persistent source cache read/write operations.
	SourceCacheStore SourceCacheStore

	AppLabels      map[string]string
	AppAnnotations map[string]string

	ClusterVersion types.ClusterVersion
	Output         interface{}
}

// SourceCacheWriteMeta carries identity and lifetime metadata stamped onto a
// persisted source cache entry so a context-free garbage-collection sweep can
// reason about it without re-evaluating the source CUE.
type SourceCacheWriteMeta struct {
	// TTL is the effective cache TTL resolved for this entry.
	TTL time.Duration
	// SourceDefName / SourceDefNamespace identify the owning SourceDefinition.
	SourceDefName      string
	SourceDefNamespace string
	// TemplateName is the ConfigTemplate the entry was rendered against, if any.
	TemplateName string

	// The fields below are the entry's identity inputs, recorded onto the object
	// so an operator can find an entry and see why it is distinct from its
	// neighbours.
	//
	// Only identity inputs are safe to record. A cache entry is shared by every
	// binding that resolves to it, so anything outside the identity - the
	// consuming application, say - differs between those sharers, and recording
	// it would describe whichever one happened to write the entry first.

	// KeyInputs names the context values that formed the identity.
	KeyInputs []string
	// Context holds those values.
	Context map[string]interface{}
	// Properties holds the binding's inputs.
	Properties map[string]interface{}
	// TemplateHash fingerprints the definition that produced the entry.
	TemplateHash string
}

// SourceCacheStore abstracts source cache persistence operations.
type SourceCacheStore interface {
	Read(ctx context.Context, cacheKey string, ttl time.Duration) (map[string]interface{}, bool, bool, time.Time, error)
	Write(ctx context.Context, cacheKey, sourceType string, data map[string]interface{}, meta SourceCacheWriteMeta) error
}

// SourceCacheToucher is optionally implemented by a SourceCacheStore to record
// that a stale entry was served. It advances a last-accessed marker only while
// an entry is still in use but no longer being refreshed, which is exactly the
// window the GC sweep must respect. Stores that cannot record this may omit it.
type SourceCacheToucher interface {
	Touch(ctx context.Context, cacheKey string) error
}

// policyAdditionalContextKey is the shared Go context key for policy output.ctx data
const policyAdditionalContextKey = oam.PolicyAdditionalContextKey

// NewContext creates a new process context
func NewContext(data ContextData) process.Context {
	// Extract policy additionalContextif it exists
	// This allows Application-scoped policies to inject data into component/trait rendering
	var customData map[string]interface{}
	if data.Ctx != nil {
		if val := data.Ctx.Value(policyAdditionalContextKey); val != nil {
			if contextMap, ok := val.(map[string]interface{}); ok {
				// Wrap additionalContext under "custom" key so it's accessible as context.custom
				customData = map[string]interface{}{
					"custom": contextMap,
				}
			}
		}
	}

	ctx := process.NewContext(process.ContextData{
		Namespace:      data.Namespace,
		Name:           data.CompName,
		StepName:       data.StepName,
		WorkflowName:   data.WorkflowName,
		PublishVersion: data.PublishVersion,
		Ctx:            data.Ctx,
		BaseHooks:      data.BaseHooks,
		AuxiliaryHooks: data.AuxiliaryHooks,
		CustomData:     customData,
	})
	ctx.PushData(ContextAppName, data.AppName)
	ctx.PushData(ContextAppRevision, data.AppRevisionName)
	ctx.PushData(ContextCompRevisionName, data.CompRevision)
	ctx.PushData(ContextComponents, data.Components)
	if data.Sources == nil {
		data.Sources = map[string]map[string]interface{}{}
	}
	ctx.PushData(ContextAppSources, data.Sources)
	if data.SourceTypes == nil {
		data.SourceTypes = map[string]string{}
	}
	ctx.PushData(ContextAppSourceTypes, data.SourceTypes)
	if data.SourceTemplates == nil {
		data.SourceTemplates = map[string]string{}
	}
	ctx.PushData(ContextAppSourceTemplates, data.SourceTemplates)
	if data.SourceSensitivePaths == nil {
		data.SourceSensitivePaths = map[string][]string{}
	}
	ctx.PushData(ContextAppSourceSensitivePaths, data.SourceSensitivePaths)
	if data.SourceCacheStore != nil {
		ctx.PushData(ContextAppSourceCacheStore, data.SourceCacheStore)
	}
	appLabels := data.AppLabels
	if appLabels == nil {
		appLabels = map[string]string{}
	}
	appAnnotations := data.AppAnnotations
	if appAnnotations == nil {
		appAnnotations = map[string]string{}
	}
	ctx.PushData(ContextAppLabels, appLabels)
	ctx.PushData(ContextAppAnnotations, appAnnotations)
	ctx.PushData(ContextReplicaKey, data.ReplicaKey)
	revNum, _ := util.ExtractRevisionNum(data.AppRevisionName, "-")
	ctx.PushData(ContextAppRevisionNum, revNum)
	ctx.PushData(ContextCluster, data.Cluster)
	ctx.PushData(ContextClusterVersion, parseClusterVersion(data.ClusterVersion))
	if data.Output != nil {
		ctx.PushData(OutputFieldName, data.Output)
	}
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
