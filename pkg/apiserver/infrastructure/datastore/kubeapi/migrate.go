/*
 Copyright 2022 The KubeVela Authors.

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

package kubeapi

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/clients"
)

// MigrateKey marks the label key of the migrated data
const MigrateKey = "db.oam.dev/migrated"

// migrate will migrate the configmap to new short table name, it won't delete the configmaps:
// users can delete by the following commands:
// kubectl -n kubevela delete cm -l db.oam.dev/migrated=ok
func migrate(dbns string) {
	kubeClient, err := clients.GetKubeClient()
	if err != nil {
		panic(err)
	}
	models := model.GetRegisterModels()
	for _, k := range models {
		var configMaps corev1.ConfigMapList
		table := k.TableName()
		selector, _ := labels.Parse(fmt.Sprintf("table=%s", table))
		if err = kubeClient.List(context.Background(), &configMaps, &client.ListOptions{Namespace: dbns, LabelSelector: selector}); err != nil {
			err = client.IgnoreNotFound(err)
			if err != nil {
				klog.Errorf("migrate db for kubeapi storage err: %v", err)
				continue
			}
		}
		var migrated bool
		for _, cm := range configMaps.Items {
			if strings.HasPrefix(cm.Name, strings.ReplaceAll(k.ShortTableName()+"-", "_", "-")) {
				migrated = true
				break
			}
		}
		if migrated || len(configMaps.Items) == 0 {
			continue
		}
		klog.Infof("migrating data for table %v", k.TableName())
		for _, cm := range configMaps.Items {
			cm := cm
			checkprefix := strings.ReplaceAll(fmt.Sprintf("veladatabase-%s", k.TableName()), "_", "-")
			if !strings.HasPrefix(cm.Name, checkprefix) {
				continue
			}

			cm.Labels[MigrateKey] = "ok"
			err = kubeClient.Update(context.Background(), &cm)
			if err != nil {
				klog.Errorf("update migrated record %s for kubeapi storage err: %v", cm.Name, err)
			}
			cm.Name = strings.ReplaceAll(k.ShortTableName()+strings.TrimPrefix(cm.Name, checkprefix), "_", "-")
			cm.ResourceVersion = ""
			delete(cm.Labels, MigrateKey)
			err = kubeClient.Create(context.Background(), &cm)
			if err != nil {
				klog.Errorf("migrate record %s for kubeapi storage err: %v", cm.Name, err)
			}
		}
	}
}
