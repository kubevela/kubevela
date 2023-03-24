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

package controller

import (
	"github.com/spf13/pflag"

	velaclient "github.com/kubevela/pkg/controller/client"
	wfContext "github.com/kubevela/workflow/pkg/context"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/cache"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
)

// AddOptimizeFlags add optimize flags
func AddOptimizeFlags(fs *pflag.FlagSet) {
	// optimize client
	fs.StringVar(&velaclient.CachedGVKs, "optimize-cached-gvks", "", "Types of resources to be cached. For example, --optimize-cached-gvks=Deployment.v1.apps,Job.v1.batch . If you have dedicated resources to be managed in your system, you can turn it on to improve performance. NOTE: this optimization only works for single-cluster.")
	fs.BoolVar(&cache.OptimizeListOp, "optimize-list-op", true, "Optimize ResourceTracker & ApplicationRevision list op by adding index. This will increase the use of memory and accelerate the list operation of ResourceTracker. Default to enable it. If you want to reduce the memory use of KubeVela, you can switch it off.")
	fs.DurationVarP(&cache.ApplicationRevisionDefinitionCachePruneDuration, "application-revision-definition-storage-prune-duration", "", cache.ApplicationRevisionDefinitionCachePruneDuration, "the duration for running application revision storage pruning")

	// optimize functions
	fs.Float64Var(&resourcekeeper.MarkWithProbability, "optimize-mark-with-prob", 0.1, "Optimize ResourceTracker GC by only run mark with probability. Side effect: outdated ResourceTracker might not be able to be removed immediately. Default to 0.1. If you want to cleanup outdated resource for keepLegacyResource mode immediately, set it to 1.0 to disable this optimization.")
	fs.BoolVar(&application.DisableAllComponentRevision, "optimize-disable-component-revision", false, "Optimize ComponentRevision by disabling the creation and gc. Side effect: rollout cannot be used. If you don't use rollout trait, you can switch it on to reduce the storage and improve performance.")
	fs.BoolVar(&application.DisableAllApplicationRevision, "optimize-disable-application-revision", false, "Optimize ApplicationRevision by disabling the creation and gc. Side effect: application cannot rollback. If you don't need to rollback applications, you can switch it on to reduce the storage and improve performance.")
	fs.BoolVar(&wfContext.EnableInMemoryContext, "optimize-enable-in-memory-workflow-context", false, "Optimize workflow by use in-memory context. Side effect: controller crash will lead to workflow run again from scratch and possible to cause mistakes in workflow inputs/outputs. You can use this optimization when you don't use input/output feature of workflow.")
	fs.BoolVar(&application.DisableResourceApplyDoubleCheck, "optimize-disable-resource-apply-double-check", false, "Optimize workflow by ignoring resource double check after apply. Side effect: controller will not wait for resource creation. If you want to use KubeVela to dispatch tons of resources and do not need to double check the creation result, you can enable this optimization.")
	fs.BoolVar(&application.EnableResourceTrackerDeleteOnlyTrigger, "optimize-enable-resource-tracker-delete-only-trigger", true, "Optimize resourcetracker by only trigger reconcile when resourcetracker is deleted. It is enabled by default. If you want to integrate KubeVela with your own operator or allow ResourceTracker manual edit, you can turn it off.")
}

// AddAdmissionFlags add admission flags
func AddAdmissionFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&resourcekeeper.AllowCrossNamespaceResource, "allow-cross-namespace-resource", true, "If set to false, application can only apply resources within its namespace. Default to be true.")
	fs.StringVar(&resourcekeeper.AllowResourceTypes, "allow-resource-types", "", "If not empty, application can only apply resources with specified types. For example, --allow-resource-types=whitelist:Deployment.v1.apps,Job.v1.batch")
	fs.StringVar(&component.RefObjectsAvailableScope, "ref-objects-available-scope", component.RefObjectsAvailableScopeGlobal, "The available scope for ref-objects component to refer objects. Should be one of `namespace`, `cluster`, `global`")

	// auth flags
	fs.BoolVar(&auth.AuthenticationWithUser, "authentication-with-user", false, "If set to true, User will be carried on application. Resource requests will be impersonated as the User.")
	fs.StringVar(&auth.AuthenticationDefaultUser, "authentication-default-user", types.KubeVelaName+":"+types.VelaCoreName, "The User to impersonate when the User of application is not set.")
	fs.StringVar(&auth.AuthenticationGroupPattern, "authentication-group-pattern", auth.DefaultAuthenticateGroupPattern, "During authentication, only groups with specified pattern will be carried on application. Resource requests will be impersonated as these selected groups.")
}
