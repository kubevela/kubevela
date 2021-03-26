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

package standard_oam_dev

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/webhook/standard.oam.dev/v1alpha1/podspecworkload"
)

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-standard-oam-dev-v1alpha1-metricstrait,mutating=false,failurePolicy=fail,groups=standard.oam.dev,resources=metricstraits,versions=v1alpha1,name=vmetricstrait.kb.io
// +kubebuilder:webhook:path=/mutate-standard-oam-dev-v1alpha1-metricstrait,mutating=true,failurePolicy=fail,groups=standard.oam.dev,resources=metricstraits,verbs=create;update,versions=v1alpha1,name=mmetricstrait.kb.io
// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-standard-oam-dev-v1alpha1-podspecworkload,mutating=false,failurePolicy=fail,groups=standard.oam.dev,resources=PodSpecWorkload,versions=v1alpha1,name=vpodspecworkload.kb.io
// +kubebuilder:webhook:path=/mutate-standard-oam-dev-v1alpha1-podspecworkload,mutating=true,failurePolicy=fail,groups=standard.oam.dev,resources=PodSpecWorkload,verbs=create;update,versions=v1alpha1,name=mpodspecworkload.kb.io

// Register will register all the services to the webhook server
func Register(mgr manager.Manager, disableCaps string) {
	disableCapsSet := utils.StoreInSet(disableCaps)
	server := mgr.GetWebhookServer()
	if disableCaps == common.DisableNoneCaps || !disableCapsSet.Contains(common.PodspecWorkloadControllerName) {
		// PodSpecWorkload
		server.Register("/validate-standard-oam-dev-v1alpha1-podspecworkload",
			&webhook.Admission{Handler: &podspecworkload.ValidatingHandler{}})
		server.Register("/mutate-standard-oam-dev-v1alpha1-podspecworkload",
			&webhook.Admission{Handler: &podspecworkload.MutatingHandler{}})
	}
}
