// +build integration

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

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/controller"
	v1alph2controller "github.com/crossplane/oam-kubernetes-runtime/pkg/controller/v1alpha2"
)

var (
	errUnexpectedSubstitution = "unexpected substitution value"
	errUnexpectedContainers   = "unexpected containers in containerizedworkload"

	defaultNS      = "default"
	cwName         = "test-cw"
	compName       = "test-component"
	wdName         = "containerizedworkloads.core.oam.dev"
	containerName  = "test-container"
	containerImage = "notarealimage"
	acName         = "test-ac"

	envVars = []string{
		"VAR_ONE",
		"VAR_TWO",
		"VAR_THREE",
	}

	paramVals = []string{
		"replace-one",
		"replace-two",
		"replace-three",
	}
)

func TestAppConfigController(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		test   func(c client.Client) error
	}{
		{
			name:   "ApplicationConfigurationRendersWorkloads",
			reason: "An ApplicationConfiguration should render its workloads.",
			test: func(c client.Client) error {
				d := wd(wdNameAndDef(wdName))

				if err := c.Create(context.Background(), d); err != nil {
					return err
				}

				workload := cw(
					cwWithName(cwName),
					cwWithContainers([]v1alpha2.Container{
						{
							Name:  containerName,
							Image: containerImage,
							Environment: []v1alpha2.ContainerEnvVar{
								{
									Name: envVars[0],
								},
								{
									Name: envVars[1],
								},
								{
									Name: envVars[2],
								},
							},
							Ports: []v1alpha2.ContainerPort{
								{
									Name: "http",
									Port: 8080,
								},
							},
						},
					}),
				)

				rawWorkload := runtime.RawExtension{Object: workload}

				co := comp(
					compWithName(compName),
					compWithNamespace(defaultNS),
					compWithWorkload(rawWorkload),
					compWithParams([]v1alpha2.ComponentParameter{
						{
							Name:       envVars[0],
							FieldPaths: []string{"spec.containers[0].env[0].value"},
						},
						{
							Name:       envVars[1],
							FieldPaths: []string{"spec.containers[0].env[1].value"},
						},
						{
							Name:       envVars[2],
							FieldPaths: []string{"spec.containers[0].env[2].value"},
						},
					}))

				if err := c.Create(context.Background(), co); err != nil {
					return err
				}

				ac := ac(
					acWithName(acName),
					acWithNamspace(defaultNS),
					acWithComps([]v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: compName,
							ParameterValues: []v1alpha2.ComponentParameterValue{
								{
									Name:  envVars[0],
									Value: intstr.FromString(paramVals[0]),
								},
								{
									Name:  envVars[1],
									Value: intstr.FromString(paramVals[1]),
								},
								{
									Name:  envVars[2],
									Value: intstr.FromString(paramVals[2]),
								},
							},
						},
					}))

				if err := c.Create(context.Background(), ac); err != nil {
					return err
				}

				if err := waitFor(context.Background(), 3*time.Second, func() (bool, error) {
					cw := &v1alpha2.ContainerizedWorkload{}
					if err := c.Get(context.Background(), types.NamespacedName{Name: cwName, Namespace: defaultNS}, cw); err != nil {
						if kerrors.IsNotFound(err) {
							return false, nil
						}
						return false, err
					}

					if len(cw.Spec.Containers) != 1 {
						return true, errors.New(errUnexpectedContainers)
					}
					for i, e := range cw.Spec.Containers[0].Environment {
						if e.Name != envVars[i] {
							return true, errors.New(errUnexpectedSubstitution)
						}
						if e.Value != nil && *e.Value != paramVals[i] {
							return true, errors.New(errUnexpectedSubstitution)
						}
					}

					return true, nil
				}); err != nil {
					return err
				}

				if err := c.Delete(context.Background(), ac); err != nil {
					return err
				}

				err := waitFor(context.Background(), 3*time.Second, func() (bool, error) {
					cw := &v1alpha2.ContainerizedWorkload{}
					if err := c.Get(context.Background(), types.NamespacedName{Name: cwName, Namespace: defaultNS}, cw); err != nil {
						if kerrors.IsNotFound(err) {
							return true, nil
						}
						return false, err
					}

					return false, nil
				})

				return err
			},
		},
	}

	cfg, err := ctrl.GetConfig()
	if err != nil {
		t.Fatal(err)
	}

	i, err := integration.New(cfg,
		integration.WithCRDPaths("../../charts/oam-kubernetes-runtime/crds"),
		integration.WithCleaners(
			integration.NewCRDCleaner(),
			integration.NewCRDDirCleaner()),
	)

	if err != nil {
		t.Fatal(err)
	}

	if err := core.AddToScheme(i.GetScheme()); err != nil {
		t.Fatal(err)
	}

	if err := corev1.AddToScheme(i.GetScheme()); err != nil {
		t.Fatal(err)
	}

	if err := apiextensionsv1beta1.AddToScheme(i.GetScheme()); err != nil {
		t.Fatal(err)
	}

	zl := zap.New(zap.UseDevMode(true))
	log := logging.NewLogrLogger(zl.WithName("app-config"))
	if err := v1alph2controller.Setup(i, controller.Args{RevisionLimit: 10}, log); err != nil {
		t.Fatal(err)
	}

	i.Run()

	defer func() {
		if err := i.Cleanup(); err != nil {
			t.Fatal(err)
		}
	}()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.test(i.GetClient())
			if err != nil {
				t.Error(err)
			}
		})
	}
}
