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

package testutil

import (
	"context"
	"time"

	"github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcileRetry will reconcile with retry
func ReconcileRetry(r reconcile.Reconciler, req reconcile.Request) {
	gomega.Eventually(func() error {
		if _, err := r.Reconcile(context.TODO(), req); err != nil {
			return err
		}
		return nil
	}, 15*time.Second, time.Second).Should(gomega.BeNil())
}

// ReconcileRetryAndExpectErr will reconcile and get error
func ReconcileRetryAndExpectErr(r reconcile.Reconciler, req reconcile.Request) {
	gomega.Eventually(func() error {
		if _, err := r.Reconcile(context.TODO(), req); err != nil {
			return err
		}
		return nil
	}, 3*time.Second, time.Second).ShouldNot(gomega.BeNil())
}

// ReconcileOnce will just reconcile once
func ReconcileOnce(r reconcile.Reconciler, req reconcile.Request) {
	//nolint:errcheck
	r.Reconcile(context.TODO(), req)
}

// ReconcileOnceAfterFinalizer will reconcile for finalizer
//nolint:errcheck
func ReconcileOnceAfterFinalizer(r reconcile.Reconciler, req reconcile.Request) (reconcile.Result, error) {
	// 1st and 2nd time reconcile to add finalizer
	r.Reconcile(context.TODO(), req)
	r.Reconcile(context.TODO(), req)

	return r.Reconcile(context.TODO(), req)
}
