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

package apply

import (
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// Config holds the behaviour the host binary decides for this package. Switches are
// functions, evaluated on every call, so a host can back them with feature gates that
// change at runtime. A nil switch takes the default documented on it, which matches
// KubeVela's gate default.
type Config struct {
	// ReplaceOnUpdate updates an existing updatable resource by replacing it rather than
	// patching it, unless the call sets a strategy. Default false
	// (KubeVela: ApplyResourceByReplace).
	ReplaceOnUpdate func() bool
	// LegacyOwnerValidation lets an existing resource with no owner be adopted without
	// take-over. Default false (KubeVela: LegacyResourceOwnerValidation).
	LegacyOwnerValidation func() bool
	// Scheme resolves the kind of a typed object passed without TypeMeta. Default:
	// client-go's types plus core.oam.dev.
	Scheme *runtime.Scheme
	// DefaultOwnerKind is the kind of owner keys written without one (<namespace>/<name>,
	// or <name>), from before owner kinds existed. Default "": such keys are compared as
	// written (KubeVela: Application).
	DefaultOwnerKind string
	// Migration: owner labels (see pkg/resourcekeeper's package doc).
	// LegacyControlledBy reads marks older than owner.oam.dev/* labels and returns their
	// owner's key ("" for nobody). Ownership checks consult it only when the owner's own
	// ControlledBy finds nobody, so a resource not yet re-labelled is still its owner's.
	// Default: none (KubeVela: app.oam.dev/* labels, as Application/<namespace>/<name>).
	LegacyControlledBy func(client.Object) string
}

var config atomic.Pointer[Config]

var defaultScheme = func() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = v1alpha1.AddToScheme(s)
	_ = v1beta1.AddToScheme(s)
	return s
}()

// Configure sets this package's Config for the process. Hosts call it at start-up.
func Configure(c Config) {
	config.Store(&c)
}

// CurrentConfig returns the Config in effect (the zero Config if Configure was never called).
func CurrentConfig() Config {
	return current()
}

func current() Config {
	if c := config.Load(); c != nil {
		return *c
	}
	return Config{}
}

func enabled(f func() bool) bool {
	return f != nil && f()
}

func replaceOnUpdate() bool { return enabled(current().ReplaceOnUpdate) }

func legacyOwnerValidation() bool { return enabled(current().LegacyOwnerValidation) }

// controllerOf is who controls existing: the owner's own view, else the legacy marks.
func controllerOf(existing client.Object, controlledBy func(client.Object) string) string {
	if by := controlledBy(existing); by != "" {
		return by
	}
	if f := current().LegacyControlledBy; f != nil {
		return f(existing)
	}
	return ""
}

func scheme() *runtime.Scheme {
	if s := current().Scheme; s != nil {
		return s
	}
	return defaultScheme
}
