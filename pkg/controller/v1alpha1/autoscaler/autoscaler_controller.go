/*


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

package controllers

import (
	"context"
	"flag"
	"path/filepath"
	"reflect"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/client-go/util/homedir"

	kedav1alpha1 "github.com/kedacore/keda/api/v1alpha1"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/go-logr/logr"
	"github.com/oam-dev/kubevela/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	restclient "k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SpecWarningTargetWorkloadNotSet = "Spec.targetWorkload is not set"
	SpecWarningStartAtTimeFormat    = "startAt is not in the right format, which should be like `12:01`"
)

var (
	scaledObjectKind       = reflect.TypeOf(kedav1alpha1.ScaledObject{}).Name()
	scaledObjectAPIVersion = "keda.k8s.io/v1alpha1"
)

// ReconcileWaitResult is the time to wait between reconciliation.
var ReconcileWaitResult = reconcile.Result{RequeueAfter: 30 * time.Second}

// AutoscalerReconciler reconciles a Autoscaler object
type AutoscalerReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	record event.Recorder
	config *restclient.Config
	ctx    context.Context
}

// +kubebuilder:rbac:groups=standard.oam.dev,resources=autoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=standard.oam.dev,resources=autoscalers/status,verbs=get;update;patch
func (r *AutoscalerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("autoscaler", req.NamespacedName)
	log.Info("Reconciling Autoscaler...")

	var scaler v1alpha1.Autoscaler
	if err := r.Get(r.ctx, req.NamespacedName, &scaler); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Autoscaler is deleted")
		}
		return ReconcileWaitResult, client.IgnoreNotFound(err)
	}
	log.Info("retrieved trait Autoscaler", "APIVersion", scaler.APIVersion, "Kind", scaler.Kind)

	// find ApplicationConfiguration to record the event
	// comment it as I don't want Autoscaler to know it's in OAM context
	//eventObj, err := util.LocateParentAppConfig(ctx, r.Client, &scaler)
	//if err != nil {
	//	log.Error(err, "failed to locate ApplicationConfiguration", "AutoScaler", scaler.Name)
	//}

	namespace := req.NamespacedName.Namespace

	// TODO(zzxwill) compare two scalers and adjust the target replicas @炎寻

	if err := r.scaleByHPA(scaler, namespace, log); err != nil {
		return ReconcileWaitResult, err
	}

	if err := r.scaleByKEDA(scaler, namespace, log); err != nil {
		return ReconcileWaitResult, err
	}

	return ctrl.Result{}, nil
}

func (r *AutoscalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.buildConfig(); err != nil {
		return err
	}
	r.ctx = context.Background()
	r.record = event.NewAPIRecorder(mgr.GetEventRecorderFor("Autoscaler")).
		WithAnnotations("controller", "Autoscaler")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Autoscaler{}).
		Complete(r)
}

func (r *AutoscalerReconciler) buildConfig() error {
	var kubeConfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeConfig = flag.String("kubeConfig", filepath.Join(home, ".kube", "config"), "kubeConfig file")
	}
	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", *kubeConfig)
	if err != nil {
		return err
	}
	r.config = config
	return nil
}
