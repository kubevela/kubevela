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

package cuetest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func hookNames(hooks []*Hook) []string {
	var names []string
	for _, h := range hooks {
		names = append(names, h.Name)
	}
	return names
}

func TestHooksLoad(t *testing.T) {
	suites, err := Load("testdata/hooks")
	require.NoError(t, err)
	s := suites[0]
	require.NoError(t, s.Err)
	require.Equal(t, []string{"platform", "shared"}, hookNames(s.Before))
	require.Equal(t, []string{"seed"}, hookNames(s.BeforeEach))
	require.Equal(t, []string{"tidy"}, hookNames(s.AfterEach))
	require.Equal(t, []string{"report"}, hookNames(s.After))
	require.Len(t, s.Cases, 2)
}

// writeHookSuite writes a test file beside copies of the publish step and
// the web component.
func writeHookSuite(t *testing.T, src string) string {
	dir := t.TempDir()
	step, err := os.ReadFile("testdata/steps/publish.cue")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "publish.cue"), step, 0o600))
	web, err := os.ReadFile("testdata/defs/web.cue")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "web.cue"), web, 0o600))
	file := filepath.Join(dir, "hooks_test.cue")
	imports := "import \"vela/test\"\n"
	if strings.Contains(src, "kube.") {
		imports = "import (\n\t\"vela/test\"\n\t\"vela/kube\"\n)\n"
	}
	require.NoError(t, os.WriteFile(file, []byte(imports+src), 0o600))
	return file
}

const publishCase = `
"publishes": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "mine", data: {}}
}
`

func TestHooksLoadErrors(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"no function": {
			src:  `setup: {a: 1} @before()` + publishCase,
			want: `hook "setup": calls no provider function`,
		},
		"two hooks": {
			src:  `setup: test.#Seed & {$params: objects: []} @before() @after()` + publishCase,
			want: `hook "setup": marked both @before and @after`,
		},
		"a case": {
			src:  strings.Replace(publishCase, "test.#WorkflowStepExec & {", "test.#WorkflowStepExec & {\n@before()", 1),
			want: `case "publishes": a case cannot be a hook`,
		},
		"a nested mark": {
			src:  `outer: {inner: test.#Seed & {$params: objects: []} @before()}` + publishCase,
			want: `outer.inner: @before marks a nested field; mark the top-level field instead`,
		},
		"a mark inside a hook": {
			src:  `setup: {inner: test.#Seed & {$params: objects: []} @afterEach()} @before()` + publishCase,
			want: `setup.inner: @afterEach marks a nested field; mark the top-level field instead`,
		},
		"a nested mark in a case": {
			src:  strings.Replace(publishCase, `parameter: {`, `parameter: x: {} @before()`+"\n\tparameter: {", 1),
			want: `publishes.parameter.x: @before marks a nested field`,
		},
		"no exec case": {
			src: `setup: test.#Seed & {$params: objects: []} @before()
"renders": test.#ComponentRender & {definition: "web", parameter: image: "nginx"}`,
			want: `hooks run against the cluster only step and source cases have`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			suites, err := Load(writeHookSuite(t, tc.src))
			require.NoError(t, err)
			require.ErrorContains(t, suites[0].Err, tc.want)
		})
	}
}

func TestHooksRun(t *testing.T) {
	cl := testCluster(t)
	suites, err := Load("testdata/hooks")
	require.NoError(t, err)
	s := suites[0]
	require.NoError(t, s.Err)
	for i, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures, s.Cases[i].Name)
	}
	for _, key := range []client.ObjectKey{
		{Namespace: "platform", Name: "shared"},
		{Namespace: "default", Name: "unscoped"},
		{Namespace: "platform", Name: "per-case"},
		{Namespace: "platform", Name: "mine"},
		{Namespace: "default", Name: "report"},
	} {
		err := cl.Client.Get(context.Background(), key, &corev1.ConfigMap{})
		require.True(t, apierrors.IsNotFound(err), "%s: want deleted, got %v", key, err)
	}
	require.NoError(t, cl.Client.Get(context.Background(), client.ObjectKey{Name: "platform"}, &corev1.Namespace{}),
		"namespaces are never deleted: envtest cannot finish deleting one")
}

func TestHookCleanup(t *testing.T) {
	cl := testCluster(t)
	ctx := context.Background()
	gone := func(t *testing.T, name string) bool {
		err := cl.Client.Get(ctx, client.ObjectKey{Namespace: "default", Name: name}, &corev1.ConfigMap{})
		if apierrors.IsNotFound(err) {
			return true
		}
		require.NoError(t, err)
		return false
	}
	cm := func(name string) string {
		return `{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "` + name + `", namespace: "default"}}`
	}
	bad := `test.#Seed & {$params: objects: [{apiVersion: "v1", kind: "Nonsense", metadata: name: "x"}]}`

	t.Run("what a failing @before created before it failed", func(t *testing.T) {
		s := loadOne(t, writeHookSuite(t, `setup: {
	a: test.#Seed & {$params: objects: [`+cm("partial-a")+`]}
	b: kube.#Apply & {$params: value: `+cm("partial-b")+`}
	c: `+bad+`
} @before()
`+publishCase))
		require.NotEmpty(t, s.Evaluate(s.Cases, RunOptions{})[0].Failures)
		require.True(t, gone(t, "partial-a"))
		require.True(t, gone(t, "partial-b"))
	})
	t.Run("not what a hook only updated", func(t *testing.T) {
		pre := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "preexisting", Namespace: "default"}}
		require.NoError(t, cl.Client.Create(ctx, pre))
		t.Cleanup(func() { require.NoError(t, cl.Client.Delete(ctx, pre)) })
		s := loadOne(t, writeHookSuite(t, `setup: kube.#Apply & {$params: value: `+cm("preexisting")+` & {data: touched: "yes"}} @beforeEach()
`+publishCase))
		require.Empty(t, s.Evaluate(s.Cases, RunOptions{})[0].Failures)
		require.False(t, gone(t, "preexisting"))
	})
	t.Run("a case's own resources, after the case", func(t *testing.T) {
		s := loadOne(t, writeHookSuite(t, strings.Replace(publishCase, `parameter: {`, `resources: [`+cm("case-seeded")+`]
	parameter: {`, 1)))
		require.Empty(t, s.Evaluate(s.Cases, RunOptions{})[0].Failures)
		require.True(t, gone(t, "case-seeded"))
	})
}

func TestHooksNested(t *testing.T) {
	cl := testCluster(t)
	seedCM := func(name, data string) string {
		return `test.#Seed & {$params: objects: [{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "` + name + `", namespace: "default"}` + data + `}]}`
	}
	s := loadOne(t, writeHookSuite(t, `
setup: {
	a: `+seedCM("nested-a", `, data: from: "a"`)+`
	group: inner: {
		read: kube.#Read & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "nested-a", namespace: "default"}}}
		b: `+seedCM("nested-b", ", data: read.$returns.value.data")+`
	}
	_c: `+seedCM("nested-c", "")+`
} @before()

_hidden: `+seedCM("hidden-hook", "")+` @before()

"sees every nested call's object": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "mine", data: {}}
	expect: resources: [
		{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "nested-a", namespace: "default"}},
		// b runs after a, reading what it made.
		{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "nested-b", namespace: "default"}, data: from: "a"},
		{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "nested-c", namespace: "default"}},
		{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "hidden-hook", namespace: "default"}},
	]
}
`))
	require.Equal(t, []string{"setup", "_hidden"}, hookNames(s.Before))
	for _, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures)
	}
	err := cl.Client.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "nested-b"}, &corev1.ConfigMap{})
	require.True(t, apierrors.IsNotFound(err), "deleted after the file, got %v", err)
}

func TestHookNamespaces(t *testing.T) {
	testCluster(t)
	s := loadOne(t, writeHookSuite(t, `
each: {
	cm: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "each-unnamed"}}
	ns: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "each-cluster-scoped"}}
} @beforeEach()

once: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: name: "once-unnamed"}} @before()

"sees each in its own namespace": test.#WorkflowStepExec & {
	definition: "publish"
	parameter: {name: "mine", data: {}}
	expect: resources: [
		{apiVersion: "v1", kind: "ConfigMap", metadata: name: "each-unnamed"},
		{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "once-unnamed", namespace: "default"}},
		{apiVersion: "v1", kind: "Namespace", metadata: name: "each-cluster-scoped"},
	]
}
`))
	for _, o := range s.Evaluate(s.Cases, RunOptions{}) {
		require.Empty(t, o.Failures)
	}
}

func TestHookFailures(t *testing.T) {
	testCluster(t)
	bad := `test.#Seed & {$params: objects: [{apiVersion: "v1", kind: "Nonsense", metadata: name: "x"}]}`
	// A marker updates a ConfigMap that exists beforehand, so cleanup, which
	// deletes only what hooks create, leaves the mark; the test deletes it.
	marker := func(name string) string {
		cl, err := execCluster(".")
		require.NoError(t, err)
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
		require.NoError(t, cl.Client.Create(context.Background(), cm))
		t.Cleanup(func() { require.NoError(t, cl.Client.Delete(context.Background(), cm)) })
		return `kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "` + name + `", namespace: "default"}, data: ran: "yes"}}`
	}
	ran := func(t *testing.T, name string) bool {
		cl, err := execCluster(".")
		require.NoError(t, err)
		cm := &corev1.ConfigMap{}
		require.NoError(t, cl.Client.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, cm))
		return cm.Data["ran"] == "yes"
	}
	second := strings.Replace(publishCase, `"publishes"`, `"publishes again"`, 1)

	t.Run("@before fails every case, and @after still runs", func(t *testing.T) {
		s := loadOne(t, writeHookSuite(t, "setup: "+bad+" @before()\nafter: "+marker("after-ran-1")+" @after()\n"+publishCase+second))
		for _, o := range s.Evaluate(s.Cases, RunOptions{}) {
			require.Len(t, o.Failures, 1)
			require.Contains(t, o.Failures[0], `@before "setup": `)
		}
		require.True(t, ran(t, "after-ran-1"))
	})
	t.Run("@beforeEach fails its case without running the step; @afterEach still runs", func(t *testing.T) {
		s := loadOne(t, writeHookSuite(t, "setup: "+bad+" @beforeEach()\nafter: "+marker("after-ran-2")+" @afterEach()\n"+publishCase))
		o := s.Evaluate(s.Cases, RunOptions{})
		require.Len(t, o[0].Failures, 1)
		require.Contains(t, o[0].Failures[0], `@beforeEach "setup": `)
		require.True(t, ran(t, "after-ran-2"))
	})
	t.Run("@after fails the last case", func(t *testing.T) {
		s := loadOne(t, writeHookSuite(t, "teardown: "+bad+" @after()\n"+publishCase+second))
		o := s.Evaluate(s.Cases, RunOptions{})
		require.Empty(t, o[0].Failures)
		require.Len(t, o[1].Failures, 1)
		require.Contains(t, o[1].Failures[0], `@after "teardown": `)
	})
	t.Run("no cases to run, no hooks", func(t *testing.T) {
		s := loadOne(t, writeHookSuite(t, "setup: "+marker("never")+" @before()\n"+publishCase))
		require.Empty(t, s.Evaluate(nil, RunOptions{}))
		require.False(t, ran(t, "never"))
	})
}

func loadOne(t *testing.T, file string) *Suite {
	suites, err := Load(file)
	require.NoError(t, err)
	require.NoError(t, suites[0].Err)
	return suites[0]
}

// What a step creates is deleted after its case, like everything else the
// case created, so a later case, or an @upgrade retry, starts clean.
func TestStepCreationsAreCleanedUp(t *testing.T) {
	cl := testCluster(t)
	s := loadOne(t, writeHookSuite(t, `"publishes into a fixed namespace": test.#WorkflowStepExec & {
	definition: "publish"
	context: namespace: "step-cleanup"
	parameter: {name: "made-by-the-step", data: {}}
	expect: resources: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "made-by-the-step"}]
}
`))
	require.Empty(t, s.Evaluate(s.Cases, RunOptions{})[0].Failures)
	err := cl.Client.Get(context.Background(), client.ObjectKey{Namespace: "step-cleanup", Name: "made-by-the-step"}, &corev1.ConfigMap{})
	require.True(t, apierrors.IsNotFound(err), "deleted after the case, got %v", err)
}
