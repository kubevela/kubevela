/*
Copyright 2020 The Crossplane Authors.

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

package healthscope

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller"
)

const (
	reconcileTimeout = 1 * time.Minute
	longWait         = 10 * time.Second
)

// Reconcile error strings.
const (
	errGetHealthScope          = "cannot get health scope"
	errUpdateHealthScopeStatus = "cannot update health scope status"
)

// Reconcile event reasons.
const (
	reasonHealthCheck = "HealthCheck"
)

// Setup adds a controller that reconciles HealthScope.
func Setup(mgr ctrl.Manager, args controller.Args, l logging.Logger) error {
	name := "oam/" + strings.ToLower(v1alpha2.HealthScopeGroupKind)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha2.HealthScope{}).
		Complete(NewReconciler(mgr,
			WithLogger(l.WithValues("controller", name)),
			WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		))
}

// A Reconciler reconciles OAM Scopes by keeping track of the health status of components.
type Reconciler struct {
	client client.Client

	log    logging.Logger
	record event.Recorder
	// traitChecker represents checker fetching health condition from HealthCheckTrait
	traitChecker WorloadHealthChecker
	// checkers represents a set of built-in checkers
	checkers []WorloadHealthChecker
	// unknownChecker represents checker handling workloads that
	// cannot be hanlded by traitChecker nor built-in checkers
	unknownChecker WorloadHealthChecker
}

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// WithTraitChecker adds health checker based on HealthCheckTrait
func WithTraitChecker(c WorloadHealthChecker) ReconcilerOption {
	return func(r *Reconciler) {
		r.traitChecker = c
	}
}

// WithChecker adds workload health checker
func WithChecker(c WorloadHealthChecker) ReconcilerOption {
	return func(r *Reconciler) {
		if r.checkers == nil {
			r.checkers = make([]WorloadHealthChecker, 0)
		}
		r.checkers = append(r.checkers, c)
	}
}

// NewReconciler returns a Reconciler that reconciles HealthScope by keeping track of its healthstatus.
func NewReconciler(m ctrl.Manager, o ...ReconcilerOption) *Reconciler {
	r := &Reconciler{
		client:       m.GetClient(),
		log:          logging.NewNopLogger(),
		record:       event.NewNopRecorder(),
		traitChecker: WorkloadHealthCheckFn(CheckByHealthCheckTrait),
		checkers: []WorloadHealthChecker{
			WorkloadHealthCheckFn(CheckPodSpecWorkloadHealth),
			WorkloadHealthCheckFn(CheckContainerziedWorkloadHealth),
			WorkloadHealthCheckFn(CheckDeploymentHealth),
			WorkloadHealthCheckFn(CheckStatefulsetHealth),
			WorkloadHealthCheckFn(CheckDaemonsetHealth),
		},
		unknownChecker: WorkloadHealthCheckFn(CheckUnknownWorkload),
	}
	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile an OAM HealthScope by keeping track of its health status.
func (r *Reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	hs := &v1alpha2.HealthScope{}
	if err := r.client.Get(ctx, req.NamespacedName, hs); err != nil {
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetHealthScope)
	}

	interval := longWait
	if hs.Spec.ProbeInterval != nil {
		interval = time.Duration(*hs.Spec.ProbeInterval) * time.Second
	}

	if interval <= 0 {
		interval = longWait
	}

	start := time.Now()

	log = log.WithValues("uid", hs.GetUID(), "version", hs.GetResourceVersion())

	scopeCondition, wlConditions := r.GetScopeHealthStatus(ctx, hs)
	log.Debug("Successfully ran health check", "scope", hs.Name)
	r.record.Event(hs, event.Normal(reasonHealthCheck, "Successfully ran health check"))

	elapsed := time.Since(start)
	hs.Status.ScopeHealthCondition = scopeCondition
	hs.Status.WorkloadHealthConditions = wlConditions

	return reconcile.Result{RequeueAfter: interval - elapsed}, errors.Wrap(r.client.Status().Update(ctx, hs), errUpdateHealthScopeStatus)
}

// GetScopeHealthStatus get the status of the healthscope based on workload resources.
func (r *Reconciler) GetScopeHealthStatus(ctx context.Context, healthScope *v1alpha2.HealthScope) (ScopeHealthCondition, []*WorkloadHealthCondition) {
	log := r.log.WithValues("get scope health status", healthScope.GetName())
	scopeCondition := ScopeHealthCondition{
		HealthStatus: StatusHealthy, // if no workload referenced, scope is healthy by default
	}
	scopeWLRefs := healthScope.Spec.WorkloadReferences
	if len(scopeWLRefs) == 0 {
		return scopeCondition, []*WorkloadHealthCondition{}
	}

	timeout := defaultTimeout
	if healthScope.Spec.ProbeTimeout != nil {
		timeout = time.Duration(*healthScope.Spec.ProbeTimeout) * time.Second
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// process workloads concurrently
	workloadHealthConditionsC := make(chan *WorkloadHealthCondition, len(scopeWLRefs))
	var wg sync.WaitGroup
	wg.Add(len(scopeWLRefs))

	for _, workloadRef := range scopeWLRefs {
		go func(resRef runtimev1alpha1.TypedReference) {
			defer wg.Done()
			var wlHealthCondition *WorkloadHealthCondition

			wlHealthCondition = r.traitChecker.Check(ctx, r.client, resRef, healthScope.GetNamespace())
			if wlHealthCondition != nil {
				log.Debug("get health condition from health check trait ", "workload", resRef, "healthCondition", wlHealthCondition)
				// get healthCondition from HealthCheckTrait
				workloadHealthConditionsC <- wlHealthCondition
				return
			}

			for _, checker := range r.checkers {
				wlHealthCondition = checker.Check(ctxWithTimeout, r.client, resRef, healthScope.GetNamespace())
				if wlHealthCondition != nil {
					log.Debug("get health condition from built-in checker", "workload", resRef, "healthCondition", wlHealthCondition)
					// found matched checker and get health condition
					workloadHealthConditionsC <- wlHealthCondition
					return
				}
			}
			// handle unknown workload
			log.Debug("get unknown workload", "workload", resRef)
			workloadHealthConditionsC <- r.unknownChecker.Check(ctx, r.client, resRef, healthScope.GetNamespace())
		}(workloadRef)
	}

	go func() {
		wg.Wait()
		close(workloadHealthConditionsC)
	}()

	var healthyCount, unhealthyCount, unknownCount int64
	workloadHealthConditions := []*WorkloadHealthCondition{}
	for wlC := range workloadHealthConditionsC {
		workloadHealthConditions = append(workloadHealthConditions, wlC)
		switch wlC.HealthStatus { //nolint:exhaustive
		case StatusHealthy:
			healthyCount++
		case StatusUnhealthy:
			unhealthyCount++
		case StatusUnknown:
			unknownCount++
		default:
			unknownCount++
		}
	}
	if unhealthyCount > 0 || unknownCount > 0 {
		// ANY unhealthy or unknown worloads make the whole scope unhealthy
		scopeCondition.HealthStatus = StatusUnhealthy
	}
	scopeCondition.Total = int64(len(scopeWLRefs))
	scopeCondition.HealthyWorkloads = healthyCount
	scopeCondition.UnhealthyWorkloads = unhealthyCount
	scopeCondition.UnknownWorkloads = unknownCount

	return scopeCondition, workloadHealthConditions
}
