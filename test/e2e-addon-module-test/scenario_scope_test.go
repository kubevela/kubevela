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

package addonmoduletest

import (
	"context"
	"os"
	"reflect"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	veltypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	pkgmodule "github.com/oam-dev/kubevela/pkg/module"
	moduleservice "github.com/oam-dev/kubevela/pkg/module/service"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestScenarioScopeIdentities(t *testing.T) {
	s := newScenarioScope("abcdef0123456789", "02", t.TempDir())
	for _, tc := range []struct{ original, want string }{
		{"default", "am-s02-abcdef0123456789"},
		{"widget-kit", "widget-kit-s02-abcdef0123456789"},
		{"module-widget-kit", "module-widget-kit-s02-abcdef0123456789"},
		{"widget-kit/v1/widget", "widget-kit-s02-abcdef0123456789/v1/widget-s02-abcdef0123456789"},
		{"widget-kit-v1-widget", "widget-kit-s02-abcdef0123456789-v1-widget-s02-abcdef0123456789"},
		{"widget-kit-v1-standard", "widget-kit-s02-abcdef0123456789-v1-standard"},
		{"widgets.kit.example.com", "widgets.s02-abcdef0123456789.kit.example.com"},
		{"kit.example.com/v1alpha1", "s02-abcdef0123456789.kit.example.com/v1alpha1"},
		{"default/platform-a", "am-s02-abcdef0123456789/platform-a"},
		{"vela-system/addon-widget-platform", "vela-system/addon-widget-platform-s02-abcdef0123456789"},
		{"/widget-platform", "/widget-platform-s02-abcdef0123456789"},
		{"e2e-addons/widget-platform", "e2e-addons/widget-platform-s02-abcdef0123456789"},
		{`type "widget" is ambiguous`, `type "widget-s02-abcdef0123456789" is ambiguous`},
		{"kit.example.com/labeled-by", "kit.example.com/labeled-by"},
		{"module", "module"},
		{"Ready:7/7", "Ready:7/7"},
		{"color", "color"},
	} {
		if got := s.Text(tc.original); got != tc.want {
			t.Errorf("Text(%q) = %q, want %q", tc.original, got, tc.want)
		}
	}
	if other := newScenarioScope("abcdef0123456789", "03", t.TempDir()); other.Text("note") == s.Text("note") || other.Namespace == s.Namespace || other.Group == s.Group {
		t.Fatal("scenarios share a namespace, API group or exported short name")
	}
	if other := newScenarioScope("fedcba9876543210", "02", t.TempDir()); other.Text("widget-kit") == s.Text("widget-kit") {
		t.Fatal("overlapping runs share a module identity")
	}
}

func TestScopedModuleFixturesPreserveTopologyAndResolution(t *testing.T) {
	root := t.TempDir()
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	for _, id := range []string{"02", "03"} {
		s := newScenarioScope("abcdef0123456789", id, root)
		if err := s.Materialize(testdataPath()); err != nil {
			t.Fatal(err)
		}
		mod, err := pkgmodule.ParseModuleDir(s.Path("modules", "widget-kit-1.0.0"))
		if err != nil {
			t.Fatal(err)
		}
		if mod.Name != "widget-kit-s"+id+"-abcdef0123456789" || mod.Version != "1.0.0" || len(mod.Lines) != 3 {
			t.Fatalf("fixture identity/version/line topology changed: %+v", mod)
		}
		app, err := moduleservice.RenderApplication(mod, "vela-system")
		if err != nil {
			t.Fatal(err)
		}
		spec := app["spec"].(map[string]interface{})
		components := spec["components"].([]interface{})
		if len(components) != 5 {
			t.Fatalf("enabled tier count = %d, want 5", len(components))
		}
		for _, component := range components {
			objects := component.(map[string]interface{})["properties"].(map[string]interface{})["objects"].([]interface{})
			for _, raw := range objects {
				obj := raw.(map[string]interface{})
				if obj["kind"] != "ComponentDefinition" && obj["kind"] != "TraitDefinition" {
					continue
				}
				if obj["kind"] == "TraitDefinition" {
					var td v1beta1.TraitDefinition
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj, &td); err != nil {
						t.Fatal(err)
					}
					if err := cli.Create(context.Background(), &td); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	for _, id := range []string{"02", "03"} {
		ctx := oamutil.SetNamespaceInCtx(context.Background(), "vela-system")
		note := "note-s" + id + "-abcdef0123456789"
		got, err := appfile.ResolveModuleType(ctx, cli, note, veltypes.TypeTrait)
		if err != nil || got != "widget-kit-s"+id+"-abcdef0123456789-v1-"+note {
			t.Fatalf("scoped Form 1 note = %q, %v", got, err)
		}
		if _, err := appfile.ResolveModuleType(ctx, cli, "labeler-s"+id+"-abcdef0123456789", veltypes.TypeTrait); err == nil {
			t.Fatal("intentional ambiguity across the two API lines was lost")
		}
	}
}

func TestScopedFixturesPreserveBrokenImportsAndInterpolation(t *testing.T) {
	s := newScenarioScope("abcdef0123456789", "14", t.TempDir())
	if err := s.Materialize(testdataPath()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bad-name", "bad-semver", "duplicate-line", "no-lines"} {
		original, originalErr := pkgmodule.ParseModuleDir(testdataPath("modules", "invalid", name))
		copy, copyErr := pkgmodule.ParseModuleDir(s.Path("modules", "invalid", name))
		if !reflect.DeepEqual(original, copy) || originalErr == nil || copyErr == nil || originalErr.Error() != copyErr.Error() {
			t.Fatalf("invalid fixture %s was changed: %v vs %v", name, originalErr, copyErr)
		}
	}
	data, err := os.ReadFile(s.Path("addons", "broken-imports-1.0.5", "modules", "_imports.cue"))
	if err != nil {
		t.Fatal(err)
	}
	v := cuecontext.New().CompileBytes(data)
	imports := v.LookupPath(cue.ParsePath("imports"))
	list, err := imports.List()
	if err != nil || !list.Next() {
		t.Fatalf("broken import no longer parses: %v", err)
	}
	sources := list.Value().LookupPath(cue.ParsePath("sources"))
	iter, err := sources.List()
	count := 0
	for err == nil && iter.Next() {
		count++
	}
	if count != 2 {
		t.Fatalf("broken import source count = %d, want 2", count)
	}
	mod, err := pkgmodule.ParseModuleDir(s.Path("modules", "widget-kit-1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range mod.Lines["v1"].Definitions {
		if definition["kind"] != "TraitDefinition" {
			continue
		}
		var td v1beta1.TraitDefinition
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(definition, &td); err != nil {
			t.Fatal(err)
		}
		if td.Name != "note-s14-abcdef0123456789" {
			continue
		}
		v := cuecontext.New().CompileString(td.Spec.Schematic.CUE.Template + "\ncontext: name: \"demo\"\n")
		got, err := v.LookupPath(cue.ParsePath("outputs.note.metadata.name")).String()
		if err != nil || got != "demo-note" {
			t.Fatalf("CUE output interpolation changed: %q, %v", got, err)
		}
		return
	}
	t.Fatal("scoped note definition was not found")
}

func TestScenarioCleanupPreservesUnownedNamespace(t *testing.T) {
	gomega.RegisterTestingT(t)
	s := newScenarioScope("abcdef0123456789", "02", t.TempDir())
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: s.Namespace}},
		&v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "do-not-delete", Namespace: s.Namespace}},
	).Build()
	previous := k8sClient
	k8sClient = cli
	t.Cleanup(func() { k8sClient = previous })
	s.cleanup(context.Background())
	if err := cli.Get(context.Background(), client.ObjectKey{Namespace: s.Namespace, Name: "do-not-delete"}, &v1beta1.Application{}); err != nil {
		t.Fatalf("cleanup removed another run's Application: %v", err)
	}
}
