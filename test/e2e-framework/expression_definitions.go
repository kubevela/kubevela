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

package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// OptIn marks an Application as wanting $( ) property expressions.
//
// The suites run the controller in opt-in mode - expressions enabled, but only
// for Applications that ask - because that is the configuration a cluster should
// actually run. Running it wide open would leave the annotation path untested and
// would read $(VAR) in every Application on the cluster, which is what the gate
// exists to avoid.
func OptIn(app *v1beta1.Application) *v1beta1.Application {
	anns := app.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[oam.AnnotationCelExpressions] = "true"
	app.SetAnnotations(anns)
	return app
}

// ExprTraitDefinition builds a TraitDefinition that applies to every workload.
func ExprTraitDefinition(namespace, name, template string) *v1beta1.TraitDefinition {
	return &v1beta1.TraitDefinition{
		TypeMeta:   metav1.TypeMeta{Kind: "TraitDefinition", APIVersion: "core.oam.dev/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1beta1.TraitDefinitionSpec{
			AppliesToWorkloads: []string{"*"},
			Schematic:          &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: template}},
		},
	}
}

// ApplyDefinition creates a definition and waits until the controller has acted
// on it.
//
// A definition is written through the API server but read back by the webhook
// through the manager's cache, and admission is synchronous. Lose that race and
// an Application referring to a definition applied milliseconds earlier is
// refused outright - "WorkloadDefinition ... not found" from a definition that
// plainly exists - rather than requeued. In a negative spec it is worse than a
// flake: the wrong denial arrives and the assertion reads as a real regression.
//
// Each of these controllers stamps status once it has seen the object, so that
// status is direct evidence the cache holds it. Cheaper and more honest than a
// sleep, which would be both slower and still racy.
func ApplyDefinition(ctx context.Context, cli client.Client, obj client.Object) {
	ginkgo.GinkgoHelper()
	gomega.Expect(cli.Create(ctx, obj)).Should(gomega.SatisfyAny(gomega.BeNil(), &util.AlreadyExistMatcher{}))

	key := client.ObjectKeyFromObject(obj)
	gomega.Eventually(func() error {
		switch obj.(type) {
		case *v1beta1.SourceDefinition:
			latest := &v1beta1.SourceDefinition{}
			if err := cli.Get(ctx, key, latest); err != nil {
				return err
			}
			// The ConfigTemplate, not merely the revision: source validation
			// reads the schema through it.
			if latest.Status.ConfigTemplateRef == nil {
				return fmt.Errorf("SourceDefinition %s has no ConfigTemplate yet", key.Name)
			}
		case *v1beta1.ComponentDefinition:
			latest := &v1beta1.ComponentDefinition{}
			if err := cli.Get(ctx, key, latest); err != nil {
				return err
			}
			if latest.Status.LatestRevision == nil {
				return fmt.Errorf("ComponentDefinition %s not reconciled yet", key.Name)
			}
		case *v1beta1.PolicyDefinition:
			latest := &v1beta1.PolicyDefinition{}
			if err := cli.Get(ctx, key, latest); err != nil {
				return err
			}
			if latest.Status.LatestRevision == nil {
				return fmt.Errorf("PolicyDefinition %s not reconciled yet", key.Name)
			}
		case *v1beta1.TraitDefinition:
			latest := &v1beta1.TraitDefinition{}
			if err := cli.Get(ctx, key, latest); err != nil {
				return err
			}
			if latest.Status.LatestRevision == nil {
				return fmt.Errorf("TraitDefinition %s not reconciled yet", key.Name)
			}
		}
		return nil
	}, 60*time.Second, time.Second).Should(gomega.Succeed())
}
