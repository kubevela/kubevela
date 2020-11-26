package containerizedworkload

import (
	"context"
	"reflect"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

func TestContainerizedWorkloadReconciler_cleanupResources(t *testing.T) {
	type args struct {
		ctx        context.Context
		workload   *v1alpha2.ContainerizedWorkload
		deployUID  *types.UID
		serviceUID *types.UID
	}
	testCases := map[string]struct {
		reconciler Reconciler
		args       args
		wantErr    bool
	}{
		// TODO: Add test cases.
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			if err := testCase.reconciler.cleanupResources(testCase.args.ctx, testCase.args.workload, testCase.args.deployUID,
				testCase.args.serviceUID); (err != nil) != testCase.wantErr {
				t.Errorf("cleanupResources() error = %v, wantErr %v", err, testCase.wantErr)
			}
		})
	}
}

func TestRenderDeployment(t *testing.T) {
	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)

	r := Reconciler{
		Client: nil,
		log:    ctrl.Log.WithName("ContainerizedWorkload"),
		record: nil,
		Scheme: scheme,
	}

	cwLabel := map[string]string{
		"oam.dev/enabled": "true",
	}
	dmLabel := cwLabel
	dmLabel[labelKey] = workloadUID
	cwAnnotation := map[string]string{
		"dapr.io/enabled": "true",
	}
	dmAnnotation := cwAnnotation

	configVolumeMountPath := "/test/path/config"
	configValue := "testValue"
	secretVolumeMountPath := "/test/path/secret"
	secretKey, secretName := "testKey", "testName"
	w := containerizedWorkload(cwWithAnnotation(cwAnnotation), cwWithLabel(cwLabel), cwWithContainer(
		v1alpha2.Container{
			ConfigFiles: []v1alpha2.ContainerConfigFile{
				{
					Path: secretVolumeMountPath,
					FromSecret: &v1alpha2.SecretKeySelector{
						Key:  secretKey,
						Name: secretName,
					},
				},
				{
					Path:  configVolumeMountPath,
					Value: &configValue,
				},
			},
		},
	))
	deploy, err := r.renderDeployment(context.Background(), w)

	if diff := cmp.Diff(nil, err, test.EquateErrors()); diff != "" {
		t.Errorf("%s\ncontainerizedWorkloadTranslator(...): -want error, +got error:\n%s", "translate", diff)
	}

	if diff := cmp.Diff(dmLabel, deploy.GetLabels()); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "pass label", diff)
	}

	if diff := cmp.Diff(dmAnnotation, deploy.GetAnnotations()); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "pass annotation", diff)
	}

	if diff := cmp.Diff(dmLabel, deploy.Spec.Template.GetLabels()); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "pass label", diff)
	}

	if len(deploy.GetOwnerReferences()) != 1 {
		t.Errorf("deplyment should have one owner reference")
	}

	dof := deploy.GetOwnerReferences()[0]
	if dof.Name != workloadName || dof.APIVersion != v1alpha2.SchemeGroupVersion.String() ||
		dof.Kind != reflect.TypeOf(v1alpha2.ContainerizedWorkload{}).Name() {
		t.Errorf("deplyment should have one owner reference pointing to the ContainerizedWorkload")
	}

	if diff := cmp.Diff(2, len(deploy.Spec.Template.Spec.Volumes)); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "render volumes", diff)
	}
	secretVM := deploy.Spec.Template.Spec.Volumes[0]
	if diff := cmp.Diff(secretName, secretVM.Secret.SecretName); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "render volumes", diff)
	}
	if diff := cmp.Diff(1, len(deploy.Spec.Template.Spec.Containers)); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "render deplyment podTemplate", diff)
	}
	c := deploy.Spec.Template.Spec.Containers[0]
	if diff := cmp.Diff(2, len(c.VolumeMounts)); diff != "" {
		t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", "render volume mount", diff)
	}

}

func TestRenderConfigMaps(t *testing.T) {
	var scheme = runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = core.AddToScheme(scheme)

	r := Reconciler{
		Client: nil,
		log:    ctrl.Log.WithName("ContainerizedWorkload"),
		record: nil,
		Scheme: scheme,
	}
	testValue := "test value"
	testContainerName := "testContainerName"
	testContainerConfigFile := v1alpha2.ContainerConfigFile{
		Path:  "/test/path/configmap",
		Value: &testValue,
	}
	w := containerizedWorkload(cwWithContainer(v1alpha2.Container{
		Name:        testContainerName,
		ConfigFiles: []v1alpha2.ContainerConfigFile{testContainerConfigFile},
	}))

	configMaps, err := r.renderConfigMaps(context.Background(), w, &appsv1.Deployment{})
	if diff := cmp.Diff(nil, err, test.EquateErrors()); diff != "" {
		t.Errorf("\nReason: %s\nrenderConfigMaps(...): -want error, +got error:\n%s", "translate into ConfigMaps", diff)
	}
	if diff := cmp.Diff(1, len(configMaps)); diff != "" {
		t.Errorf("\nReason: %s\nrenderConfigMaps(...): -want error, +got error:\n%s", "translate into ConfigMaps", diff)
	}
	cm := configMaps[0]
	expectName, _ := generateConfigMapName(testContainerConfigFile, workloadName, testContainerName)
	if diff := cmp.Diff(expectName, cm.Name); diff != "" {
		t.Errorf("\nReason: %s\ngenerateConfigMapName(...): -want error, +got error:\n%s", "translate into ConfigMaps", diff)
	}
	if diff := cmp.Diff(cm.Data["configmap"], testValue); diff != "" {
		t.Errorf("\nReason: %s\nrenderConfigMaps(...): -want error, +got error:\n%s", "translate into ConfigMaps", diff)
	}

}
