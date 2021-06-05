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

package dispatch

import "testing"

func TestExtractAppNameFromResourceTrackerName(t *testing.T) {
	testcases := []struct {
		appRevName  string
		ns          string
		wantRTName  string
		wantAppName string
	}{
		{
			appRevName:  "app-v1",
			ns:          "default",
			wantRTName:  "app-v1-default",
			wantAppName: "app",
		},
		{
			appRevName:  "app-test-v2",
			ns:          "test-ns",
			wantRTName:  "app-test-v2-test-ns",
			wantAppName: "app-test",
		},
	}

	for _, tc := range testcases {
		gotRTName := ConstructResourceTrackerName(tc.appRevName, tc.ns)
		gotAppName := ExtractAppName(gotRTName, tc.ns)
		gotAppRevName := ExtractAppRevisionName(gotRTName, tc.ns)
		if gotRTName != tc.wantRTName {
			t.Fatalf("expect resource tracker name %q but got %q", tc.wantRTName, gotRTName)
		}
		if gotAppName != tc.wantAppName {
			t.Fatalf("expect app name %q but got %q", tc.wantAppName, gotAppName)
		}
		if gotAppRevName != tc.appRevName {
			t.Fatalf("expect app revision name %q but got %q", tc.appRevName, gotAppRevName)
		}
	}
}
