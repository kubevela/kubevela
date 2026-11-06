/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileUntilOK reconciles an Application, as successive resyncs would,
// until check passes, and fails after 30s with check's last error and the
// last reconcile error.
//
// It reconciles again at once rather than on a fixed poll: the test
// reconciler reads and writes through the API server directly, so there is
// no watch or cache to wait for. It waits only when the reconciler asks to be
// requeued after a short delay (a workflow backing off), and only that long.
func reconcileUntilOK(key types.NamespacedName, check func() error) {
	GinkgoHelper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		res, reconcileErr := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: key})
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			if reconcileErr != nil {
				Fail(fmt.Sprintf("%v (last reconcile error: %v)", err, reconcileErr))
			}
			Fail(err.Error())
		}
		if d := res.RequeueAfter; d > 0 && d < time.Minute {
			time.Sleep(min(d, time.Until(deadline)))
		}
	}
}
