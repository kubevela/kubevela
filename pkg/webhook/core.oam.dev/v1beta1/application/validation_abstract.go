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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	webhookutils "github.com/oam-dev/kubevela/pkg/webhook/utils"
)

// ValidateAbstractTypes refuses an Application that names a definition marked
// abstract.
//
// An abstract definition is a base: it carries rules that whatever extends it
// must apply, so naming it directly is the way around them.
//
// Only the type the Application names is judged. Extending an abstract
// definition is the point of marking one.
func (h *ValidatingHandler) ValidateAbstractTypes(ctx context.Context, app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList

	judge := func(path *field.Path, typ string, component bool) {
		abstract, err := h.isAbstract(ctx, app, typ, component)
		switch {
		case err != nil:
			// The definition could not be read for a reason other than its
			// absence, so whether it is abstract is unknown. Admitting on an
			// unknown is how a transient error or a denied read becomes a way in.
			errs = append(errs, field.Invalid(path, typ, fmt.Sprintf(
				"whether %s is abstract could not be read: %v", typ, err)))
		case abstract:
			errs = append(errs, field.Invalid(path, typ, abstractMessage(typ, kindWord(component))))
		}
	}

	for i, comp := range app.Spec.Components {
		judge(field.NewPath("spec", "components").Index(i).Child("type"), comp.Type, true)
		for j, tr := range comp.Traits {
			judge(field.NewPath("spec", "components").Index(i).Child("traits").Index(j).Child("type"),
				tr.Type, false)
		}
	}

	return errs
}

func kindWord(component bool) string {
	if component {
		return "component"
	}
	return "trait"
}

func abstractMessage(typ, kind string) string {
	return fmt.Sprintf(
		"%s %s is abstract and cannot be used directly; name a %s that extends it",
		kind, typ, kind)
}

// isAbstract reads spec.abstract off one definition.
//
// It resolves the definition the way the render path does, so a pinned revision
// is read from that revision rather than from whatever the name points at now,
// and the namespace fallback is the one the Application itself will get. A type
// that does not exist is not abstract: that fails elsewhere, with a message that
// says so.
func (h *ValidatingHandler) isAbstract(ctx context.Context, app *v1beta1.Application, typ string, component bool) (bool, error) {
	ctx = oamutil.SetNamespaceInCtx(ctx, app.Namespace)

	// An Application may be applied in the same breath as the definitions it
	// names, and reading absence off a cache that has not caught up would report
	// a definition as missing rather than as abstract, skipping the check exactly
	// when two objects arrive together. So a lookup that finds nothing is retried
	// against the API server.
	var abstract bool
	err := webhookutils.ReadWithLiveRetry(h.Client, h.Live, func(cli client.Client) error {
		var def client.Object = &v1beta1.ComponentDefinition{}
		if !component {
			def = &v1beta1.TraitDefinition{}
		}
		if err := oamutil.GetCapabilityDefinition(ctx, cli, def, typ, app.GetAnnotations()); err != nil {
			return err
		}
		switch d := def.(type) {
		case *v1beta1.ComponentDefinition:
			abstract = d.Spec.Abstract
		case *v1beta1.TraitDefinition:
			abstract = d.Spec.Abstract
		}
		return nil
	})
	if err != nil {
		return false, ignoreNotFound(err)
	}
	return abstract, nil
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
