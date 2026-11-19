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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// These tests keep their Application-shaped cases: an owner keyed <namespace>/<name> whose
// resources carry app.oam.dev/* labels, expressed through the generic options (the
// Application kind itself lives in pkg/appkeeper).

func appKey(app *v1beta1.Application) string {
	ns := app.Namespace
	if ns == "" {
		ns = metav1.NamespaceDefault
	}
	return fmt.Sprintf("%s/%s", ns, app.GetName())
}

func appControlledBy(obj client.Object) string {
	name, ns := obj.GetLabels()[oam.LabelAppName], obj.GetLabels()[oam.LabelAppNamespace]
	if name == "" || ns == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", ns, name)
}

// appOwner is the Application as an Owner, the way pkg/appkeeper implements it.
type appOwner struct{ app *v1beta1.Application }

func (a appOwner) Kind() string                          { return "application" }
func (a appOwner) Key() string                           { return appKey(a.app) }
func (a appOwner) ControlledBy(obj client.Object) string { return appControlledBy(obj) }

func mustBeControlledByApp(app *v1beta1.Application) ApplyOption {
	return MustBeControlledBy(appOwner{app})
}

func sharedByApp(app *v1beta1.Application) ApplyOption {
	return SharedBy(appOwner{app})
}
