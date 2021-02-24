package helm

import (
	"testing"

	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
)

func TestRenderHelmReleaseAndHelmRepo(t *testing.T) {
	h := testData()
	chartValues := map[string]interface{}{
		"image": map[string]interface{}{
			"tag": "1.0.1",
		},
	}
	rls, repo, err := RenderHelmReleaseAndHelmRepo(h, "test-comp", "test-app", "test-ns", chartValues)
	if err != nil {
		t.Fatalf("want: nil, got: %v", err)
	}

	expectRls := &unstructured.Unstructured{}
	expectRls.SetGroupVersionKind(helmapi.HelmReleaseGVK)
	expectRls.SetName("test-app-test-comp")
	expectRls.SetNamespace("test-ns")
	unstructured.SetNestedMap(expectRls.Object, map[string]interface{}{
		"chart": map[string]interface{}{
			"spec": map[string]interface{}{
				"chart":   "podinfo",
				"version": "1.0.0",
				"sourceRef": map[string]interface{}{
					"kind":      "HelmRepository",
					"name":      "test-app-test-comp",
					"namespace": "test-ns",
				},
			},
		},
		"interval": "5m0s",
		"values":   map[string]interface{}{"image": map[string]interface{}{"tag": "1.0.1"}},
	}, "spec")

	if diff := cmp.Diff(expectRls, rls); diff != "" {
		t.Errorf("\n%s\nApply(...): -want , +got \n%s\n", "render HelmRelease", diff)
	}

	expectRepo := &unstructured.Unstructured{}
	expectRepo.SetGroupVersionKind(helmapi.HelmRepositoryGVK)
	expectRepo.SetName("test-app-test-comp")
	expectRepo.SetNamespace("test-ns")
	unstructured.SetNestedMap(expectRepo.Object, map[string]interface{}{
		"url":      "test.com",
		"interval": "5m0s",
	}, "spec")

	if diff := cmp.Diff(expectRepo, repo); diff != "" {
		t.Errorf("\n%s\nApply(...): -want , +got \n%s\n", "render HelmRepository", diff)
	}
}

func testData() *v1alpha2.Helm {
	rlsStr :=
		`chart:
  spec:
    chart: "podinfo"
    version: "1.0.0"`
	repoStr := `url: "test.com"`
	rlsJson, _ := yaml.YAMLToJSON([]byte(rlsStr))
	repoJson, _ := yaml.YAMLToJSON([]byte(repoStr))

	h := &v1alpha2.Helm{}
	h.Release.Raw = rlsJson
	h.Repository.Raw = repoJson
	return h
}
