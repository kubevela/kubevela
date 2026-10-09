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

package application_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// Component-read specs remain in the core suite and need this small subset of
// the source helpers now owned by test/e2e-source-test.
func optIn(app *v1beta1.Application) *v1beta1.Application {
	anns := app.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[oam.AnnotationCelExpressions] = "true"
	app.SetAnnotations(anns)
	return app
}

func exprTraitDefinition(namespace, name, template string) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "TraitDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.TraitDefinitionSpec{
			AppliesToWorkloads: []string{"*"},
			Schematic:          &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: template}},
		},
	}
}

func applyDefinition(ctx context.Context, obj client.Object) {
	GinkgoHelper()
	Expect(k8sClient.Create(ctx, obj)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	key := client.ObjectKeyFromObject(obj)
	Eventually(func() error {
		switch obj.(type) {
		case *v1beta1.SourceDefinition:
			latest := &v1beta1.SourceDefinition{}
			if err := k8sClient.Get(ctx, key, latest); err != nil {
				return err
			}
			if latest.Status.ConfigTemplateRef == nil {
				return fmt.Errorf("SourceDefinition %s has no ConfigTemplate yet", key.Name)
			}
		case *v1beta1.ComponentDefinition:
			latest := &v1beta1.ComponentDefinition{}
			if err := k8sClient.Get(ctx, key, latest); err != nil {
				return err
			}
			if latest.Status.LatestRevision == nil {
				return fmt.Errorf("ComponentDefinition %s not reconciled yet", key.Name)
			}
		case *v1beta1.TraitDefinition:
			latest := &v1beta1.TraitDefinition{}
			if err := k8sClient.Get(ctx, key, latest); err != nil {
				return err
			}
			if latest.Status.LatestRevision == nil {
				return fmt.Errorf("TraitDefinition %s not reconciled yet", key.Name)
			}
		}
		return nil
	}, 60*time.Second, time.Second).Should(Succeed())
}
