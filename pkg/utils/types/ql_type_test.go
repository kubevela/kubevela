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

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestServiceEndpoint_String(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	testCases := []struct {
		name     string
		endpoint ServiceEndpoint
		expected string
	}{
		{
			name:     "empty endpoint",
			endpoint: ServiceEndpoint{},
			expected: "-",
		},
		{
			name: "http default port",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "example.com", Port: 80, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("http")},
			},
			expected: "http://example.com",
		},
		{
			name: "https default port",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "example.com", Port: 443, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("https")},
			},
			expected: "https://example.com",
		},
		{
			name: "http non-default port",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "example.com", Port: 8080, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("http")},
			},
			expected: "http://example.com:8080",
		},
		{
			name: "https non-default port with path",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "example.com", Port: 8443, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("https"), Path: "/foo"},
			},
			expected: "https://example.com:8443/foo",
		},
		{
			name: "path is root",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "example.com", Port: 8080, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("http"), Path: "/"},
			},
			expected: "http://example.com:8080",
		},
		{
			name: "tcp protocol",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "127.0.0.1", Port: 3306, Protocol: corev1.ProtocolTCP},
			},
			expected: "127.0.0.1:3306",
		},
		{
			name: "tcp protocol with path",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "127.0.0.1", Port: 3306, Protocol: corev1.ProtocolTCP, Path: "/"},
			},
			expected: "127.0.0.1:3306",
		},
		{
			name: "app protocol overrides protocol",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "mydb", Port: 3306, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("mysql")},
			},
			expected: "mysql://mydb:3306",
		},
		{
			name: "port is zero",
			endpoint: ServiceEndpoint{
				Endpoint: Endpoint{Host: "example.com", Port: 0, Protocol: corev1.ProtocolTCP, AppProtocol: strPtr("http")},
			},
			expected: "http://example.com",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.endpoint.String())
		})
	}
}

func TestAppliedResource_GroupVersionKind(t *testing.T) {
	testCases := []struct {
		name     string
		resource AppliedResource
		expected schema.GroupVersionKind
	}{
		{
			name:     "apps resource",
			resource: AppliedResource{APIVersion: "apps/v1", Kind: "Deployment"},
			expected: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		{
			name:     "core resource",
			resource: AppliedResource{APIVersion: "v1", Kind: "Service"},
			expected: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		},
		{
			name:     "custom resource",
			resource: AppliedResource{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
			expected: schema.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "Application"},
		},
		{
			name:     "empty resource",
			resource: AppliedResource{},
			expected: schema.GroupVersionKind{Group: "", Version: "", Kind: ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.resource.GroupVersionKind())
		})
	}
}

func TestResourceTreeNode_GroupVersionKind(t *testing.T) {
	testCases := []struct {
		name     string
		node     ResourceTreeNode
		expected schema.GroupVersionKind
	}{
		{
			name:     "apps resource",
			node:     ResourceTreeNode{APIVersion: "apps/v1", Kind: "Deployment"},
			expected: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		},
		{
			name:     "core resource",
			node:     ResourceTreeNode{APIVersion: "v1", Kind: "Service"},
			expected: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"},
		},
		{
			name:     "custom resource",
			node:     ResourceTreeNode{APIVersion: "core.oam.dev/v1beta1", Kind: "Application"},
			expected: schema.GroupVersionKind{Group: "core.oam.dev", Version: "v1beta1", Kind: "Application"},
		},
		{
			name:     "empty resource",
			node:     ResourceTreeNode{},
			expected: schema.GroupVersionKind{Group: "", Version: "", Kind: ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.node.GroupVersionKind())
		})
	}
}
