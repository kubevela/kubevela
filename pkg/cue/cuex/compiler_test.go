/*
Copyright 2025 The KubeVela Authors.

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

package cuex_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kubevela/pkg/cue/cuex"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubevela/pkg/util/singleton"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
)

var testCtx = struct {
	K8sClient       client.Client
	ReturnVal       string
	CueXTestPackage string
	Namespace       string
	CueXPath        string
	ExternalFnName  string
	InputParamName  string
	OutputParamName string
}{
	ReturnVal:       "external",
	CueXTestPackage: "cuex-test-package",
	Namespace:       "default",
	CueXPath:        "cuex/ext",
	ExternalFnName:  "external",
	InputParamName:  "input",
	OutputParamName: "output",
}

func TestMain(m *testing.M) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "charts", "vela-core", "crds"),
		},
	}

	var err error
	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start envtest: %v\n", err)
		os.Exit(1)
	}
	if cfg == nil {
		fmt.Fprintf(os.Stderr, "envtest config is nil")
		os.Exit(1)
	}

	testCtx.K8sClient, err = createK8sClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create k8s Client: %v\n", err)
		os.Exit(1)
	}

	mockServer := createMockServer()
	defer mockServer.Close()

	singleton.KubeConfig.Set(cfg)

	if err = createTestPackage(mockServer.URL); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err = deleteTestPackage(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Teardown failed: %v\n", err)
			os.Exit(1)
		}
	}()

	code := m.Run()

	singleton.KubeConfig.Reload()

	if err := testEnv.Stop(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to stop envtest: %v\n", err)
		os.Exit(1)
	}

	os.Exit(code)
}

func TestWorkloadCompiler(t *testing.T) {
	testCases := map[string]struct {
		cuexEnabled            bool
		workloadTemplate       string
		params                 map[string]interface{}
		expectedObj            runtime.Object
		expectedAdditionalObjs map[string]runtime.Object
		hasCompileErr          bool
		errorString            string
	}{
		"cuex disabled with no external packages": {
			cuexEnabled:            false,
			workloadTemplate:       getWorkloadTemplate(false),
			params:                 make(map[string]interface{}),
			expectedObj:            getExpectedObj(false),
			expectedAdditionalObjs: make(map[string]runtime.Object),
			hasCompileErr:          false,
			errorString:            "",
		},
		"cuex enabled with no external packages": {
			cuexEnabled:            true,
			workloadTemplate:       getWorkloadTemplate(false),
			params:                 make(map[string]interface{}),
			expectedObj:            getExpectedObj(false),
			expectedAdditionalObjs: make(map[string]runtime.Object),
			hasCompileErr:          false,
			errorString:            "",
		},
		"cuex disabled with external packages": {
			cuexEnabled:            false,
			workloadTemplate:       getWorkloadTemplate(true),
			params:                 make(map[string]interface{}),
			expectedObj:            getExpectedObj(true),
			expectedAdditionalObjs: make(map[string]runtime.Object),
			hasCompileErr:          true,
			errorString:            "builtin package \"cuex/ext\" undefined",
		},
		"cuex enabled with external packages": {
			cuexEnabled:            true,
			workloadTemplate:       getWorkloadTemplate(true),
			params:                 make(map[string]interface{}),
			expectedObj:            getExpectedObj(true),
			expectedAdditionalObjs: make(map[string]runtime.Object),
			hasCompileErr:          false,
		},
	}

	for _, tc := range testCases {
		cuex.EnableExternalPackageForDefaultCompiler = tc.cuexEnabled
		cuex.DefaultCompiler.Reload()

		ctx := process.NewContext(process.ContextData{
			AppName:         "test-app",
			CompName:        "test-component",
			Namespace:       testCtx.Namespace,
			AppRevisionName: "test-app-v1",
			ClusterVersion:  types.ClusterVersion{Minor: "19+"},
		})

		wt := definition.NewWorkloadAbstractEngine("test-workload")
		err := wt.Complete(ctx, tc.workloadTemplate, tc.params)
		assert.Equal(t, tc.hasCompileErr, err != nil)
		if tc.hasCompileErr {
			assert.NotNil(t, err)
			assert.Contains(t, err.Error(), tc.errorString)
		} else {
			output, _ := ctx.Output()
			assert.Nil(t, err)
			assert.NotNil(t, output)
			outputObj, _ := output.Unstructured()
			assert.Equal(t, tc.expectedObj, outputObj)
		}
	}
}

func createMockServer() *httptest.Server {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+testCtx.ExternalFnName {
			http.Error(w, fmt.Sprintf("unexpected path: %s, expected: /%s", r.URL.Path, testCtx.ExternalFnName), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(fmt.Sprintf("{\"%s\": \"%s\"}", testCtx.OutputParamName, testCtx.ReturnVal)))
		if err != nil {
			return
		}
	}))
	return mockServer
}

func createTestPackage(url string) error {
	ctx := context.Background()

	packageObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cue.oam.dev/v1alpha1",
			"kind":       "Package",
			"metadata": map[string]interface{}{
				"name":      testCtx.CueXTestPackage,
				"namespace": testCtx.Namespace,
			},
			"spec": map[string]interface{}{
				"path": testCtx.CueXPath,
				"provider": map[string]interface{}{
					"endpoint": url,
					"protocol": "http",
				},
				"templates": map[string]interface{}{
					"ext/cue": strings.TrimSpace(fmt.Sprintf(`
                        package ext

                        #ExternalFunction: {
                            #do: "%s",
                            #provider: "%s",
                            $params: {
                                %s: string
                            },
                            $returns: {
                                %s: string
                            }
                        }
                    `, testCtx.ExternalFnName, testCtx.CueXTestPackage, testCtx.InputParamName, testCtx.OutputParamName)),
				},
			},
		},
	}

	err := testCtx.K8sClient.Create(ctx, packageObj)

	err = wait.PollImmediate(time.Second, 10*time.Second, func() (bool, error) {
		err = testCtx.K8sClient.Get(ctx, client.ObjectKey{
			Name:      testCtx.CueXTestPackage,
			Namespace: testCtx.Namespace,
		}, packageObj)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("failed to create test package: %w", err)
	}
	return nil
}

func deleteTestPackage() error {
	ctx := context.Background()

	testPkg := &unstructured.Unstructured{}
	testPkg.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cue.oam.dev",
		Version: "v1alpha1",
		Kind:    "Package",
	})
	testPkg.SetName(testCtx.CueXTestPackage)
	testPkg.SetNamespace(testCtx.Namespace)

	err := testCtx.K8sClient.Delete(ctx, testPkg)
	if err != nil {
		return fmt.Errorf("failed to delete test package: %w", err)
	}
	err = wait.PollImmediate(time.Second, 10*time.Second, func() (bool, error) {
		err := testCtx.K8sClient.Get(ctx, client.ObjectKey{
			Name:      testCtx.CueXTestPackage,
			Namespace: testCtx.Namespace,
		}, testPkg)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to delete test package: %w", err)
	}
	return nil
}

func getWorkloadTemplate(includeExt bool) string {
	tmpl := ""
	name := "test-deployment"
	if includeExt {
		name = "test-deployment-\\(external.$returns.output)"
		tmpl = tmpl + strings.TrimSpace(fmt.Sprintf(`
			import (
				"%s"
			)

			external: ext.#ExternalFunction & {
				$params: {
					%s: "external"
				}
			}
		`, testCtx.CueXPath, testCtx.InputParamName)) + "\n"

	}

	tmpl = tmpl + strings.TrimSpace(fmt.Sprintf(`
		output: {
			apiVersion: "apps/v1"
			kind: "Deployment"
			metadata: name: "%s"
			spec: replicas: 1
		}
	`, name))
	return tmpl
}

func getExpectedObj(includeExt bool) *unstructured.Unstructured {
	name := "test-deployment"
	if includeExt {
		name = fmt.Sprintf("test-deployment-%s", testCtx.ReturnVal)
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{"replicas": int64(1)},
	}}
}

func createK8sClient(config *rest.Config) (client.Client, error) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add corev1 to scheme: %w", err)
	}
	return client.New(config, client.Options{Scheme: scheme})
}
