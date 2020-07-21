package cmd

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	cmdtesting "k8s.io/kubectl/pkg/cmd/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	coreoamdevv1alpha2 "github.com/cloud-native-application/rudrx/api/v1alpha2"
	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

var (
	scheme = k8sRuntime.NewScheme()
)

type testResources struct {
	create []runtime.Object
	update []runtime.Object
}

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = coreoamdevv1alpha2.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func TestNewRunCommand(t *testing.T) {
	templateExample, workloaddefExample := getTestExample()

	templateExample2 := templateExample.DeepCopy()
	workloaddefExample2 := workloaddefExample.DeepCopy()
	workloaddefExample2.Annotations["short"] = "containerized"

	cases := map[string]struct {
		resources *testResources
		// want to exist with error
		wantException bool
		// output equal to
		expectedOutput string
		// output contains
		expectedString string
		args           []string
	}{
		"WorkloadNotDefinited": {
			resources: &testResources{
				create: []runtime.Object{
					workloaddefExample,
					templateExample,
				},
			},
			wantException:  true,
			expectedString: "You must specify a workload, like containerizedworkloads.core.oam.dev",
			args:           []string{},
		},
		"WorkloadShortWork": {
			resources: &testResources{
				create: []runtime.Object{
					workloaddefExample2,
					templateExample2,
				},
			},
			wantException:  true,
			expectedString: "You must specify a workload, like containerized",
			args:           []string{},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			factory := cmdtesting.NewTestFactory().WithNamespace("test")
			client := fake.NewFakeClientWithScheme(scheme)
			iostream, _, outPut, _ := cmdutil.NewTestIOStreams()

			if len(tc.resources.create) != 0 {
				for _, resource := range tc.resources.create {
					err := client.Create(context.TODO(), resource)
					assert.NilError(t, err)
				}
			}

			if len(tc.resources.update) != 0 {
				for _, resource := range tc.resources.update {
					err := client.Update(context.TODO(), resource)
					println(111, err.Error())
					assert.NilError(t, err)
				}
			}
			runCmd := NewRunCommand(factory, client, iostream, []string{})
			runCmd.SetOutput(outPut)

			err := runCmd.Execute()
			errTip := tc.expectedString
			if tc.expectedOutput != "" {
				errTip = tc.expectedOutput
			}
			if tc.wantException {
				assert.ErrorContains(t, err, errTip)
				return
			}

			if tc.expectedOutput != "" {
				assert.Equal(t, tc.expectedOutput, outPut.String())
				return
			}

			assert.Equal(t, true, strings.Contains(outPut.String(), tc.expectedString))
		})
	}
}

func getTestExample() (*coreoamdevv1alpha2.Template, *corev1alpha2.WorkloadDefinition) {
	templateExample := &coreoamdevv1alpha2.Template{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admin.oam.dev/v1alpha2",
			Kind:       "Template",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "containerizedworkload-template",
			Annotations: map[string]string{
				"version": "0.0.1",
			},
		},
		Spec: coreoamdevv1alpha2.TemplateSpec{
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1alpha2",
					"kind":       "ContainerizedWorkload",
					"metadata": map[string]interface{}{
						"name": "pod",
					},
					"spec": map[string]interface{}{
						"containers": `
        - image: myrepo/myapp:v1
					name: master
					ports:
            - containerPort: 6379
							protocol: TCP
              name: tbd`,
					},
				},
			},
			LastCommandParam: "image",
			Parameters: []coreoamdevv1alpha2.Parameter{
				coreoamdevv1alpha2.Parameter{
					Name:       "image",
					Short:      "i",
					Required:   true,
					Type:       "string",
					FieldPaths: []string{"spec.containers[0].image"},
				},
				coreoamdevv1alpha2.Parameter{
					Name:       "port",
					Short:      "p",
					Required:   false,
					Type:       "int",
					FieldPaths: []string{"spec.containers[0].ports[0].containerPort"},
				},
			},
		},
	}
	workloaddefExample := &corev1alpha2.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "WorkloadDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "containerizedworkloads.core.oam.dev",
			Annotations: map[string]string{
				"defatultTemplateRef": "containerizedworkload-template",
			},
		},
		Spec: corev1alpha2.WorkloadDefinitionSpec{
			Reference: corev1alpha2.DefinitionReference{
				Name: "containerizedworkloads.core.oam.dev",
			},
			ChildResourceKinds: []corev1alpha2.ChildResourceKind{
				corev1alpha2.ChildResourceKind{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
				},
				corev1alpha2.ChildResourceKind{
					APIVersion: "v1",
					Kind:       "Service",
				},
			},
		},
	}

	return templateExample, workloaddefExample
}
