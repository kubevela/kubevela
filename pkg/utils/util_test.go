/*
Copyright 2021 The KubeVela Authors.

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

package utils

import (
	"testing"
)

func TestRandom(t *testing.T) {
	s1 := RandomString(10)
	s2 := RandomString(10)
	if s1 == s2 {
		t.Error("random generate same string")
	}
	if len(s1) != 10 {
		t.Error("s1 length != 10")
	}
	if len(s2) != 10 {
		t.Error("s2 length != 10")
	}
}
