package cmd

import (
	"github.com/cloud-native-application/rudrx/api/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

// used in testing
var (
	workloadTemplateExample = &types.Template{

		Parameters: []types.Parameter{
			types.Parameter{
				Name:     "image",
				Short:    "i",
				Required: true,
			},
			types.Parameter{
				Name:     "port",
				Short:    "p",
				Required: false,
			},
		},
	}

	traitTemplateExample = &types.Template{

		Parameters: []types.Parameter{
			types.Parameter{
				Name:     "replicaCount",
				Short:    "i",
				Required: true,
				Default:  "5",
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
