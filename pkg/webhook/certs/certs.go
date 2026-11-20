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

// Package certs sets up the serving cert of the vela-core admission webhooks.
// It replaces the admission-create and admission-patch Jobs
// (kube-webhook-certgen) that the chart ran as Helm hooks.
package certs

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
)

// conversionReviewVersions are the versions the Application conversion webhook
// accepts, as the admission-patch Job configured them.
var conversionReviewVersions = []string{"v1beta1", "v1alpha2"}

// Options configures Ensure.
type Options struct {
	// SecretNamespace and SecretName locate the Secret holding the serving cert.
	SecretNamespace string
	SecretName      string
	// ConfigurationName is the name of both the MutatingWebhookConfiguration and
	// the ValidatingWebhookConfiguration to patch.
	ConfigurationName string
	// FailurePolicy, if not empty, goes to every webhook. The chart renders
	// Ignore until vela-core sets the caBundle, so admission does not fail
	// before then.
	FailurePolicy admissionregistrationv1.FailurePolicyType
	// ConversionCRDs are CRDs whose conversion webhook should point at the
	// webhook Service, at path /convert.
	ConversionCRDs []string
}

// Ensure makes sure that the serving cert Secret is valid. It also makes sure
// that every webhook in the configurations trusts its CA. It returns the
// serving cert and the CA bundle. You can call it periodically. It renews the
// cert when necessary and repairs a caBundle or failurePolicy that something
// changed.
func Ensure(ctx context.Context, c client.Client, opts Options) (*servingcert.Result, error) {
	configs, err := getConfigurations(ctx, c, opts.ConfigurationName)
	if err != nil {
		return nil, err
	}
	svc, err := webhookService(configs, opts.ConfigurationName)
	if err != nil {
		return nil, err
	}
	res, err := servingcert.EnsureSecret(ctx, c, opts.SecretNamespace, opts.SecretName,
		servingcert.Service{Namespace: svc.Namespace, Name: svc.Name})
	if err != nil {
		return nil, err
	}
	for _, obj := range configs {
		if err = patchConfiguration(ctx, c, obj, res.CABundle, opts.FailurePolicy); err != nil {
			return nil, err
		}
	}
	for _, name := range opts.ConversionCRDs {
		if err = patchConversion(ctx, c, name, svc, res.CABundle); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// WriteFiles writes the serving cert and key to dir, where the webhook server
// reads them. It reports whether a file changed. It does not write a file that
// already holds the same data, so the webhook server reloads only after a
// renewal.
func WriteFiles(dir string, res *servingcert.Result) (bool, error) {
	changed := false
	for name, data := range map[string][]byte{servingcert.CertKey: res.Cert, servingcert.KeyKey: res.Key} {
		wrote, err := servingcert.WriteFileIfChanged(filepath.Join(dir, name), data)
		if err != nil {
			return changed, errors.Wrap(err, "failed to write webhook cert")
		}
		changed = changed || wrote
	}
	return changed, nil
}

// getConfigurations returns the Mutating- and ValidatingWebhookConfiguration
// named name that exist.
func getConfigurations(ctx context.Context, c client.Client, name string) ([]client.Object, error) {
	var configs []client.Object
	for _, obj := range []client.Object{
		&admissionregistrationv1.MutatingWebhookConfiguration{},
		&admissionregistrationv1.ValidatingWebhookConfiguration{},
	} {
		err := c.Get(ctx, types.NamespacedName{Name: name}, obj)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get %T %s", obj, name)
		}
		configs = append(configs, obj)
	}
	return configs, nil
}

// webhookService returns the Service the webhooks in configs call.
func webhookService(configs []client.Object, name string) (*admissionregistrationv1.ServiceReference, error) {
	for _, obj := range configs {
		switch o := obj.(type) {
		case *admissionregistrationv1.MutatingWebhookConfiguration:
			for _, w := range o.Webhooks {
				if w.ClientConfig.Service != nil {
					return w.ClientConfig.Service, nil
				}
			}
		case *admissionregistrationv1.ValidatingWebhookConfiguration:
			for _, w := range o.Webhooks {
				if w.ClientConfig.Service != nil {
					return w.ClientConfig.Service, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no webhook with a service found in the webhook configurations named %s", name)
}

// patchWebhook sets the CA, and the failure policy if given, on one webhook and
// reports whether anything changed.
func patchWebhook(cc *admissionregistrationv1.WebhookClientConfig, failurePolicy **admissionregistrationv1.FailurePolicyType,
	ca []byte, policy admissionregistrationv1.FailurePolicyType) bool {
	changed := false
	if !bytes.Equal(cc.CABundle, ca) {
		cc.CABundle = ca
		changed = true
	}
	if policy != "" && (*failurePolicy == nil || **failurePolicy != policy) {
		*failurePolicy = ptr.To(policy)
		changed = true
	}
	return changed
}

// patchConfiguration sets the CA, and the failure policy if given, on every
// webhook of the webhook configuration obj.
func patchConfiguration(ctx context.Context, c client.Client, obj client.Object, ca []byte,
	policy admissionregistrationv1.FailurePolicyType) error {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			return client.IgnoreNotFound(err)
		}
		changed := false
		switch o := obj.(type) {
		case *admissionregistrationv1.MutatingWebhookConfiguration:
			for i := range o.Webhooks {
				changed = patchWebhook(&o.Webhooks[i].ClientConfig, &o.Webhooks[i].FailurePolicy, ca, policy) || changed
			}
		case *admissionregistrationv1.ValidatingWebhookConfiguration:
			for i := range o.Webhooks {
				changed = patchWebhook(&o.Webhooks[i].ClientConfig, &o.Webhooks[i].FailurePolicy, ca, policy) || changed
			}
		}
		if !changed {
			return nil
		}
		klog.InfoS("Updating webhook configuration", "kind", fmt.Sprintf("%T", obj), "name", obj.GetName(), "failurePolicy", policy)
		return c.Update(ctx, obj)
	})
	return errors.Wrapf(err, "failed to patch %T %s", obj, obj.GetName())
}

func patchConversion(ctx context.Context, c client.Client, name string, svc *admissionregistrationv1.ServiceReference, ca []byte) error {
	want := &apiextensionsv1.CustomResourceConversion{
		Strategy: apiextensionsv1.WebhookConverter,
		Webhook: &apiextensionsv1.WebhookConversion{
			ClientConfig: &apiextensionsv1.WebhookClientConfig{
				Service: &apiextensionsv1.ServiceReference{
					Namespace: svc.Namespace,
					Name:      svc.Name,
					Path:      ptr.To("/convert"),
					Port:      ptr.To(ptr.Deref(svc.Port, 443)),
				},
				CABundle: ca,
			},
			ConversionReviewVersions: conversionReviewVersions,
		},
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := c.Get(ctx, types.NamespacedName{Name: name}, crd); err != nil {
			return err
		}
		if equality.Semantic.DeepEqual(crd.Spec.Conversion, want) {
			return nil
		}
		crd.Spec.Conversion = want.DeepCopy()
		klog.InfoS("Setting conversion webhook of CRD", "crd", name)
		return c.Update(ctx, crd)
	})
	return errors.Wrapf(err, "failed to set conversion webhook of CRD %s", name)
}
