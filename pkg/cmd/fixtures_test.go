package cmd

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/cloud-native-application/rudrx/api/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha2.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// used in testing
var (
	workloadTemplateExample = &v1alpha2.Template{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admin.oam.dev/v1alpha2",
			Kind:       "Template",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "containerizedworkload-template",
			Annotations: map[string]string{
				"version": "0.0.1",
			},
			Namespace: "default",
		},
		Spec: v1alpha2.TemplateSpec{
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1alpha2",
					"kind":       "ContainerizedWorkload",
					"metadata": map[string]interface{}{
						"name": "pod",
					},
					"spec": map[string]interface{}{
						"containers": "",
					},
				},
			},
			LastCommandParam: "image",
			Parameters: []v1alpha2.Parameter{
				v1alpha2.Parameter{
					Name:       "image",
					Short:      "i",
					Required:   true,
					Type:       "string",
					FieldPaths: []string{"spec.containers[0].image"},
				},
				v1alpha2.Parameter{
					Name:       "port",
					Short:      "p",
					Required:   false,
					Type:       "int",
					FieldPaths: []string{"spec.containers[0].ports[0].containerPort"},
				},
			},
		},
	}

	traitTemplateExample = &v1alpha2.Template{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admin.oam.dev/v1alpha2",
			Kind:       "Template",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "manualscalertrait.core.oam.dev-template",
			Annotations: map[string]string{
				"version": "0.0.1",
			},
			Namespace: "default",
		},
		Spec: v1alpha2.TemplateSpec{
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "core.oam.dev/v1alpha2",
					"kind":       "ManualScalerTrait",
					"metadata": map[string]interface{}{
						"name": "pod",
					},
					"spec": map[string]interface{}{
						"replicaCount": "2",
					},
				},
			},
			Parameters: []v1alpha2.Parameter{
				v1alpha2.Parameter{
					Name:       "replicaCount",
					Short:      "i",
					Required:   true,
					Type:       "int",
					FieldPaths: []string{"spec.replicaCount"},
					Default:    "5",
				},
			},
		},
	}

	workloaddefExample = &corev1alpha2.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "WorkloadDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "containerizedworkloads.core.oam.dev",
			Annotations: map[string]string{
				"rudrx.oam.dev/template": "containerizedworkload-template",
			},
			Namespace: "default",
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

	appconfigExample = &corev1alpha2.ApplicationConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "ApplicationConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "app2060",
			Annotations: map[string]string{
				"rudrx.oam.dev/template": "containerizedworkload-template",
			},
			Namespace: "default",
		},
		Spec: corev1alpha2.ApplicationConfigurationSpec{
			Components: []corev1alpha2.ApplicationConfigurationComponent{
				corev1alpha2.ApplicationConfigurationComponent{
					ComponentName: "app2060",
				},
			},
		},
	}

	componentExample = &corev1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "app2060",
			Namespace: "default",
		},
		Spec: corev1alpha2.ComponentSpec{},
	}

	traitDefinitionExample = &corev1alpha2.TraitDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.oam.dev/v1alpha2",
			Kind:       "TraitDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manualscalertrait.core.oam.dev",
			Namespace: "default",
			Annotations: map[string]string{
				"rudrx.oam.dev/template": "manualscalertrait.core.oam.dev-template",
				"short":                  "ManualScaler",
			},
		},
		Spec: corev1alpha2.TraitDefinitionSpec{
			Reference: corev1alpha2.DefinitionReference{
				Name: "manualscalertrait.core.oam.dev",
			},
			AppliesToWorkloads: []string{"core.oam.dev/v1alpha2.ContainerizedWorkload"},
		},
	}
)
