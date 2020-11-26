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

package applicationconfiguration

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

// A ControllerHooks provide customized reconcile logic for an ApplicationConfiguration
type ControllerHooks interface {
	Exec(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, logger logging.Logger) (reconcile.Result, error)
}

// ControllerHooksFn reconciles an ApplicationConfiguration
type ControllerHooksFn func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, logger logging.Logger) (reconcile.Result, error)

// Exec the customized reconcile logic on the ApplicationConfiguration
func (fn ControllerHooksFn) Exec(ctx context.Context, ac *v1alpha2.ApplicationConfiguration, logger logging.Logger) (reconcile.Result, error) {
	return fn(ctx, ac, logger)
}
