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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	apitypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// ResolvedTemplate is a template resolved for a Config, from a ConfigTemplate CRD or a
// legacy config-template-* ConfigMap. Shared by the Config controller and validating
// webhook so both resolvers agree on what a template looks like, Sensitive included.
type ResolvedTemplate struct {
	Name      string
	Namespace string
	CUE       script.CUE
	Scope     string
	Sensitive bool
}

// ResolveConfigTemplate looks up the referenced template, CRD first, falling back to a
// legacy ConfigMap. waiting is true if the ConfigTemplate CRD exists but its schema
// hasn't reconciled yet. err is ErrTemplateNotFound if neither the CRD nor the legacy
// ConfigMap exist.
func ResolveConfigTemplate(ctx context.Context, cli client.Client, ref *configv1alpha1.ConfigTemplateReference) (tmpl *ResolvedTemplate, waiting bool, err error) {
	ns := ref.Namespace
	if ns == "" {
		ns = apitypes.DefaultKubeVelaNS
	}

	var ct configv1alpha1.ConfigTemplate
	switch err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: ref.Name}, &ct); {
	case err == nil:
		if ct.Status.Phase != configv1alpha1.ConfigTemplatePhaseAvailable {
			return nil, true, nil
		}
		return &ResolvedTemplate{
			Name:      ct.Name,
			Namespace: ns,
			CUE:       script.CUE(ct.Spec.Template),
			Scope:     string(ct.Spec.Scope),
			Sensitive: ct.Spec.Sensitive,
		}, false, nil
	case apierrors.IsNotFound(err):
	default:
		return nil, false, err
	}

	var cm corev1.ConfigMap
	cmName := TemplateConfigMapNamePrefix + ref.Name
	if err := cli.Get(ctx, client.ObjectKey{Namespace: ns, Name: cmName}, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, ErrTemplateNotFound
		}
		return nil, false, err
	}
	return &ResolvedTemplate{
		Name:      ref.Name,
		Namespace: ns,
		CUE:       script.CUE(cm.Data[SaveTemplateKey]),
		Scope:     cm.Labels[apitypes.LabelConfigScope],
		Sensitive: cm.Annotations[apitypes.AnnotationConfigSensitive] == sensitiveAnnotationValue,
	}, false, nil
}
