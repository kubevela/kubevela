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

package componentdefinition

import (
	"context"
	"fmt"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Reconciler reconciles a ComponentDefinition object
type Reconciler struct {
	client.Client
	dm     discoverymapper.DiscoveryMapper
	Scheme *runtime.Scheme
	record event.Recorder
}

// Reconcile is the main logic for ComponentDefinition controller
func (r *Reconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	definitionName := req.NamespacedName.Name
	klog.InfoS("Reconciling ComponentDefinition", "Name", definitionName, "Namespace", req.Namespace)
	ctx := context.Background()

	var componentDefinition v1alpha2.ComponentDefinition
	if err := r.Get(ctx, req.NamespacedName, &componentDefinition); err != nil {
		if kerrors.IsNotFound(err) {
			err = nil
		}
		return ctrl.Result{}, err
	}

	// this is a placeholder for finalizer here in the future
	if componentDefinition.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// if Workload.Type is not empty, means componentdefinition refer to an already existing workloaddefinition
	if componentDefinition.Spec.Workload.Type != "" {
		return ctrl.Result{}, nil
	}

	var def utils.CapabilityComponentDefinition
	def.Name = req.NamespacedName.Name
	err := def.StoreOpenAPISchema(ctx, r, req.Namespace, req.Name)
	if err != nil {
		klog.ErrorS(err, "cannot store capability in ConfigMap")
		r.record.Event(&(def.ComponentDefinition), event.Warning("cannot store capability in ConfigMap", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &(def.ComponentDefinition),
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrStoreCapabilityInConfigMap, def.Name, err)))
	}

	if err := r.Status().Update(ctx, &def.ComponentDefinition); err != nil {
		klog.ErrorS(err, "cannot update componentDefinition ConfigMapRef Field")
		r.record.Event(&(def.ComponentDefinition), event.Warning("cannot update ComponentDefinition ConfigMapRef Field", err))
		return ctrl.Result{}, nil
	}
	klog.Info("Successfully stored Capability Schema in ConfigMap")

	// if Workload.Type is empty, we need create a WorkloadDefinition
	if err := r.Get(ctx, req.NamespacedName, &v1alpha2.WorkloadDefinition{}); err == nil {
		klog.Infof("WorkloadDefinition: %s already exists", componentDefinition.Name)
		return ctrl.Result{}, nil
	}

	workloadDefinition := new(v1alpha2.WorkloadDefinition)
	newCd := componentDefinition.DeepCopy()
	if err := util.ConvertComponentDef2WorkloadDef(newCd, workloadDefinition); err != nil {
		klog.ErrorS(err, "cannot convert ComponentDefinition")
		r.record.Event(&componentDefinition, event.Warning("cannot convert ComponentDefinition", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &componentDefinition,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrConvertComponentDefinition, componentDefinition.Name, err)))
	}
	owners := []metav1.OwnerReference{{
		APIVersion: v1alpha2.SchemeGroupVersion.String(),
		Kind:       v1alpha2.ComponentDefinitionKind,
		Name:       componentDefinition.Name,
		UID:        componentDefinition.UID,
		Controller: pointer.BoolPtr(true),
	}}
	workloadDefinition.SetOwnerReferences(owners)
	if err := r.Create(ctx, workloadDefinition); err != nil {
		klog.ErrorS(err, "cannot create converted WorkloadDefinition")
		r.record.Event(&componentDefinition, event.Warning("cannot create converted Workload", err))
		return ctrl.Result{}, util.PatchCondition(ctx, r, &componentDefinition,
			cpv1alpha1.ReconcileError(fmt.Errorf(util.ErrCreateConvertedWorklaodDefinition, workloadDefinition.Name, err)))
	}

	klog.InfoS("Successfully create WorkloadDefinition", "name", workloadDefinition.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager will setup with event recorder
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("ComponentDefinition")).
		WithAnnotations("controller", "ComponentDefinition")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha2.ComponentDefinition{}).
		Complete(r)
}

// Setup adds a controller that reconciles ComponentDefinition.
func Setup(mgr ctrl.Manager, _ controller.Args, _ logging.Logger) error {
	dm, err := discoverymapper.New(mgr.GetConfig())
	if err != nil {
		return err
	}
	r := Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		dm:     dm,
	}
	return r.SetupWithManager(mgr)
}
