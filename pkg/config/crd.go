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

package config

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// IsCRDTypeMissing reports whether err means a config.oam.dev type is not served
// by the API server or not registered in the client scheme. Dual-backend code
// treats that the same as a CR that does not exist: a cluster that was never
// upgraded has legacy objects and nothing else.
func IsCRDTypeMissing(err error) bool {
	return meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
}

// SetDistributionOwner makes the Config the controller owner of its distribution
// Application, so deleting the Config (via the CLI, kubectl, or GitOps removing the
// manifest) always recalls the distributed resources instead of orphaning the
// Application and the copies it manages in target namespaces/clusters.
// A config may still be legacy (a plain Secret, created by an older CLI or by the
// Nacos interim path), and the cluster may not serve the Config type at all, so a
// missing Config CR here is expected and not an error - there's just no owner to attach.
func SetDistributionOwner(ctx context.Context, cli client.Client, ns, configName, distributionName string) error {
	cfg := &configv1alpha1.Config{}
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: configName}, cfg); err != nil {
		if apierrors.IsNotFound(err) || IsCRDTypeMissing(err) {
			return nil
		}
		return fmt.Errorf("failed to load config %s to own its distribution: %w", configName, err)
	}
	// the Application controller reconciles (and updates status on) the distribution
	// concurrently, so the update below can lose an optimistic-concurrency race; retry.
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		app := &v1beta1.Application{}
		if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: distributionName}, app); err != nil {
			return err
		}
		if metav1.IsControlledBy(app, cfg) {
			return nil
		}
		if err := controllerutil.SetControllerReference(cfg, app, cli.Scheme()); err != nil {
			return err
		}
		return cli.Update(ctx, app)
	})
	if err != nil {
		return fmt.Errorf("failed to set owner reference on distribution %s: %w", distributionName, err)
	}
	return nil
}
