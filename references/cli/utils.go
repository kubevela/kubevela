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

package cli

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func getPodNameForResource(ctx context.Context, clientSet kubernetes.Interface, resourceName string, resourceNamespace string) (string, error) {
	podList, err := clientSet.CoreV1().Pods(resourceNamespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return "", err
	}
	var pods []string
	for _, p := range podList.Items {
		if strings.HasPrefix(p.Name, resourceName) {
			pods = append(pods, p.Name)
		}
	}
	if len(pods) < 1 {
		return "", fmt.Errorf("no pods found created by resource %s", resourceName)
	}
	return common.AskToChooseOnePods(pods)
}

func getCompNameFromClusterObjectReference(ctx context.Context, k8sClient client.Client, r *common2.ClusterObjectReference) (string, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(r.GroupVersionKind())
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: r.Name}, u); err != nil {
		return "", err
	}
	labels := u.GetLabels()
	if labels == nil {
		return "", nil
	}
	// Addon observability --> some Helm typed components --> some fluxcd objects --> some services. Those services
	// are not labeled with oam.LabelAppComponent
	if r.Name == common.AddonObservabilityGrafanaSvc {
		return r.Name, nil
	}
	return labels[oam.LabelAppComponent], nil
}
