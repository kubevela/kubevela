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

package resourcetracker

import (
	"bytes"
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

func TestResourceTreePrintOption_getWidthForDetails(t *testing.T) {
	testCases := []struct {
		name      string
		options   *ResourceTreePrintOptions
		colsWidth []int
		expected  int
	}{
		{
			name:      "nil max width",
			options:   &ResourceTreePrintOptions{},
			colsWidth: nil,
			expected:  math.MaxInt,
		},
		{
			name:      "sufficient width",
			options:   &ResourceTreePrintOptions{MaxWidth: ptr.To(50 + applyTimeWidth)},
			colsWidth: []int{10, 10},
			expected:  30,
		},
		{
			name:      "insufficient width",
			options:   &ResourceTreePrintOptions{MaxWidth: ptr.To(50 + applyTimeWidth)},
			colsWidth: []int{20, 20},
			expected:  math.MaxInt,
		},
		{
			name:      "no columns",
			options:   &ResourceTreePrintOptions{MaxWidth: ptr.To(50 + applyTimeWidth)},
			colsWidth: []int{},
			expected:  50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			result := tc.options._getWidthForDetails(tc.colsWidth)
			r.Equal(tc.expected, result)
		})
	}
}

func TestResourceTreePrintOptions_wrapDetails(t *testing.T) {
	r := require.New(t)
	options := &ResourceTreePrintOptions{}
	detail := "test-key: test-val\ttest-data: test-val\ntest-next-line: text-next-value  test-long-key: test long long long long value  test-append: test-append-val"
	r.Equal(
		[]string{
			"test-key: test-val test-data: test-val",
			"test-next-line: text-next-value",
			"test-long-key: test long long long long ",
			"value  test-append: test-append-val",
		},
		options._wrapDetails(detail, 40))
}

func TestBuildResourceRow(t *testing.T) {
	r := require.New(t)

	cases := map[string]struct {
		Cluster                   string
		ResourceRowStatus         string
		ExpectedCluster           string
		ExpectedResourceRowStatus string
	}{
		"localCluster": {
			Cluster:                   "",
			ResourceRowStatus:         resourceRowStatusUpdated,
			ExpectedCluster:           multicluster.ClusterLocalName,
			ExpectedResourceRowStatus: resourceRowStatusUpdated,
		},
		"remoteCluster": {
			Cluster:                   "remoteCluster",
			ResourceRowStatus:         resourceRowStatusUpdated,
			ExpectedCluster:           "remoteCluster",
			ExpectedResourceRowStatus: resourceRowStatusUpdated,
		},
	}

	for name, c := range cases {
		mr := v1beta1.ManagedResource{
			ClusterObjectReference: apicommon.ClusterObjectReference{
				Cluster: c.Cluster,
			},
		}
		rr := buildResourceRow(mr, c.ResourceRowStatus)
		r.Equal(c.ExpectedCluster, rr.mr.Cluster, name)
		r.Equal(c.ExpectedResourceRowStatus, rr.status, name)
	}

}

// newManagedResource is a helper for creating a ManagedResource for tests
func newManagedResource(cluster, namespace, kind, name string) v1beta1.ManagedResource {
	return v1beta1.ManagedResource{
		ClusterObjectReference: apicommon.ClusterObjectReference{
			Cluster: cluster,
			ObjectReference: corev1.ObjectReference{
				Namespace: namespace,
				Kind:      kind,
				Name:      name,
			},
		},
	}
}

func TestLoadResourceRows(t *testing.T) {
	r := require.New(t)
	options := &ResourceTreePrintOptions{}

	mr1 := newManagedResource("c1", "ns1", "Deployment", "d1")
	mr2 := newManagedResource("c1", "ns1", "Service", "s1")
	mr3 := newManagedResource("c2", "ns2", "ConfigMap", "cm1")
	mrDeleted := newManagedResource("c1", "ns1", "Secret", "sec1")
	mrDeleted.Deleted = true

	currentRT := &v1beta1.ResourceTracker{
		Spec: v1beta1.ResourceTrackerSpec{
			ManagedResources: []v1beta1.ManagedResource{mr1, mrDeleted},
		},
	}
	historyRTs := []*v1beta1.ResourceTracker{{
		Spec: v1beta1.ResourceTrackerSpec{
			ManagedResources: []v1beta1.ManagedResource{mr1, mr2, mr3},
		},
	}}

	rows := options.loadResourceRows(currentRT, historyRTs)
	r.Len(rows, 3)

	statusMap := map[string]string{}
	for _, row := range rows {
		statusMap[row.mr.ResourceKey()] = row.status
	}

	r.Equal(resourceRowStatusUpdated, statusMap[mr1.ResourceKey()])
	r.Equal(resourceRowStatusOutdated, statusMap[mr2.ResourceKey()])
	r.Equal(resourceRowStatusOutdated, statusMap[mr3.ResourceKey()])
	_, exists := statusMap[mrDeleted.ResourceKey()]
	r.False(exists, "Deleted resource should not be loaded")
}

func TestSortRows(t *testing.T) {
	r := require.New(t)
	options := &ResourceTreePrintOptions{}

	rows := []*resourceRow{
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c2", ObjectReference: corev1.ObjectReference{Name: "res1"}}}},
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c1", ObjectReference: corev1.ObjectReference{Namespace: "ns2", Name: "res2"}}}},
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c1", ObjectReference: corev1.ObjectReference{Namespace: "ns1", Name: "res3"}}}},
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c1", ObjectReference: corev1.ObjectReference{Namespace: "ns2", Name: "res1"}}}},
	}

	options.sortRows(rows)

	r.Equal("c1", rows[0].mr.Cluster)
	r.Equal("ns1", rows[0].mr.Namespace)

	r.Equal("c1", rows[1].mr.Cluster)
	r.Equal("ns2", rows[1].mr.Namespace)
	r.Equal("res1", rows[1].mr.Name)

	r.Equal("c1", rows[2].mr.Cluster)
	r.Equal("ns2", rows[2].mr.Namespace)
	r.Equal("res2", rows[2].mr.Name)

	r.Equal("c2", rows[3].mr.Cluster)
}

func TestAddNonExistingPlacementToRows(t *testing.T) {
	r := require.New(t)
	options := &ResourceTreePrintOptions{}

	rows := []*resourceRow{
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "cluster-a"}}},
	}
	placements := []v1alpha1.PlacementDecision{
		{Cluster: "cluster-a"},
		{Cluster: "cluster-b"},
	}

	newRows := options.addNonExistingPlacementToRows(placements, rows)
	r.Len(newRows, 2)
	r.Equal("cluster-a", newRows[0].mr.Cluster)
	r.Equal("cluster-b", newRows[1].mr.Cluster)
	r.Equal(resourceRowStatusNotDeployed, newRows[1].status)
}

// MockClusterNameMapper is a mock for testing
type MockClusterNameMapper struct{}

func (m MockClusterNameMapper) GetClusterName(cluster string) string { return cluster }

func TestFillResourceRows(t *testing.T) {
	r := require.New(t)
	options := &ResourceTreePrintOptions{ClusterNameMapper: MockClusterNameMapper{}}

	rows := []*resourceRow{
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c1", ObjectReference: corev1.ObjectReference{Namespace: "ns1", Kind: "Deployment", Name: "d1"}}}},
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c1", ObjectReference: corev1.ObjectReference{Namespace: "ns1", Kind: "Service", Name: "s1"}}}},
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c1", ObjectReference: corev1.ObjectReference{Namespace: "ns2", Kind: "ConfigMap", Name: "cm1"}}}},
		{mr: &v1beta1.ManagedResource{ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: "c2", ObjectReference: corev1.ObjectReference{Namespace: "ns1", Kind: "Ingress", Name: "i1"}}}},
	}
	colsWidth := make([]int, 4)
	options.fillResourceRows(rows, colsWidth)

	// Check blanking
	r.Equal("c1", rows[0].cluster)
	r.Equal("ns1", rows[0].namespace)
	r.Equal("", rows[1].cluster, "cluster should be blanked for same group")
	r.Equal("", rows[1].namespace, "namespace should be blanked for same group")
	r.Equal("", rows[2].cluster, "cluster should be blanked for same group")
	r.Equal("ns2", rows[2].namespace, "namespace should not be blanked for different ns")
	r.Equal("c2", rows[3].cluster, "cluster should not be blanked for different cluster")

	// Check connectors for row 1 (second item in ns1)
	r.True(rows[1].connectClusterUp)
	r.True(rows[0].connectClusterDown)
	r.True(rows[1].connectNamespaceUp)
	r.True(rows[0].connectNamespaceDown)

	// Check connectors for row 2 (first item in ns2)
	r.True(rows[2].connectClusterUp)
	r.True(rows[1].connectClusterDown)
	r.False(rows[2].connectNamespaceUp)
	r.False(rows[1].connectNamespaceDown)
}

// mockRoundTripper for testing http clients
type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.roundTripFunc != nil {
		return m.roundTripFunc(req)
	}
	return &http.Response{StatusCode: http.StatusOK}, nil
}

func TestPrintResourceTree(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	mr1 := newManagedResource("c1", "ns1", "Deployment", "d1")
	mr2 := newManagedResource("c1", "ns1", "Service", "s1")

	testCases := []struct {
		name               string
		options            *ResourceTreePrintOptions
		placements         []v1alpha1.PlacementDecision
		currentRT          *v1beta1.ResourceTracker
		historyRT          []*v1beta1.ResourceTracker
		expectedSubstrings []string
	}{
		{
			name:               "simple case with current and history",
			options:            &ResourceTreePrintOptions{ClusterNameMapper: MockClusterNameMapper{}},
			currentRT:          &v1beta1.ResourceTracker{Spec: v1beta1.ResourceTrackerSpec{ManagedResources: []v1beta1.ManagedResource{mr1}}},
			historyRT:          []*v1beta1.ResourceTracker{{Spec: v1beta1.ResourceTrackerSpec{ManagedResources: []v1beta1.ManagedResource{mr1, mr2}}}},
			expectedSubstrings: []string{"CLUSTER", "NAMESPACE", "RESOURCE", "STATUS", "c1", "ns1", "Deployment/d1", "updated", "Service/s1", "outdated"},
		},
		{
			name:               "with not-deployed placement",
			options:            &ResourceTreePrintOptions{ClusterNameMapper: MockClusterNameMapper{}},
			placements:         []v1alpha1.PlacementDecision{{Cluster: "c2"}},
			expectedSubstrings: []string{"c2", "not-deployed"},
		},
		{
			name: "with detail retriever",
			options: &ResourceTreePrintOptions{
				ClusterNameMapper: MockClusterNameMapper{},
				DetailRetriever: func(row *resourceRow, format string) error {
					row.applyTime = "2021-01-01"
					row.details = "detail-info"
					return nil
				},
			},
			currentRT:          &v1beta1.ResourceTracker{Spec: v1beta1.ResourceTrackerSpec{ManagedResources: []v1beta1.ManagedResource{mr1}}},
			expectedSubstrings: []string{"APPLY_TIME", "DETAIL", "2021-01-01", "detail-info"},
		},
		{
			name: "with detail retriever error",
			options: &ResourceTreePrintOptions{
				ClusterNameMapper: MockClusterNameMapper{},
				DetailRetriever: func(row *resourceRow, format string) error {
					return fmt.Errorf("mock error")
				},
			},
			currentRT:          &v1beta1.ResourceTracker{Spec: v1beta1.ResourceTrackerSpec{ManagedResources: []v1beta1.ManagedResource{mr1}}},
			expectedSubstrings: []string{"Error: mock error"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			var buf bytes.Buffer
			tc.options.PrintResourceTree(&buf, tc.placements, tc.currentRT, tc.historyRT)
			output := buf.String()
			for _, sub := range tc.expectedSubstrings {
				r.Contains(output, sub)
			}
		})
	}
}

func TestTableRoundTripper(t *testing.T) {
	r := require.New(t)

	var capturedReq *http.Request

	mockRT := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			capturedReq = req
			return &http.Response{StatusCode: http.StatusOK}, nil
		},
	}

	tableRT := tableRoundTripper{rt: mockRT}

	req, err := http.NewRequest("GET", "http://localhost", nil)
	r.NoError(err)

	_, err = tableRT.RoundTrip(req)
	r.NoError(err)

	r.NotNil(capturedReq)
	expectedAccept := "application/json;as=Table;v=v1;g=meta.k8s.io,application/json"
	r.Equal(expectedAccept, capturedReq.Header.Get("Accept"))
}
