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

package controllers_test

import (
	"os"
	"testing"

	"github.com/oam-dev/kubevela/pkg/utils/system"
)

func TestModuleWorkerHomeIsPrivateAndRestored(t *testing.T) {
	original := t.TempDir()
	t.Setenv(system.VelaHomeEnv, original)
	firstCleanup, err := prepareModuleWorkerHome()
	if err != nil {
		t.Fatal(err)
	}
	first := os.Getenv(system.VelaHomeEnv)
	if first == "" || first == original {
		t.Fatalf("first worker reused VELA_HOME %q", first)
	}
	secondCleanup, err := prepareModuleWorkerHome()
	if err != nil {
		t.Fatal(err)
	}
	second := os.Getenv(system.VelaHomeEnv)
	if second == first {
		t.Fatal("two workers reused a VELA_HOME directory")
	}
	if err := secondCleanup(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(system.VelaHomeEnv); got != first {
		t.Fatalf("nested cleanup restored %q, want %q", got, first)
	}
	if _, err := os.Stat(second); err == nil {
		t.Fatalf("second worker directory %s remains after cleanup", second)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking second worker directory %s: %v", second, err)
	}
	if err := firstCleanup(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(system.VelaHomeEnv); got != original {
		t.Fatalf("cleanup changed the original VELA_HOME to %q", got)
	}
	if _, err := os.Stat(first); err == nil {
		t.Fatalf("first worker directory %s remains after cleanup", first)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking first worker directory %s: %v", first, err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("original VELA_HOME was removed: %v", err)
	}
}

func TestModuleWorkerHomeUnsetsVelaHomeItSet(t *testing.T) {
	t.Setenv(system.VelaHomeEnv, "")
	if err := os.Unsetenv(system.VelaHomeEnv); err != nil {
		t.Fatal(err)
	}
	cleanup, err := prepareModuleWorkerHome()
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv(system.VelaHomeEnv)
	if home == "" {
		t.Fatal("worker home was not set")
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if got, ok := os.LookupEnv(system.VelaHomeEnv); ok {
		t.Fatalf("cleanup left VELA_HOME set to %q; it was unset before", got)
	}
	if _, err := os.Stat(home); err == nil {
		t.Fatalf("worker directory %s remains after cleanup", home)
	} else if !os.IsNotExist(err) {
		t.Fatalf("checking worker directory %s: %v", home, err)
	}
}
