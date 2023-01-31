/*
Copyright 2023 The KubeVela Authors.

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

package sharding

import (
	"github.com/kubevela/pkg/util/k8s"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetScheduledShardID return the scheduled shard-id of the object and if it is scheduled
func GetScheduledShardID(o client.Object) (string, bool) {
	ls := o.GetLabels()
	if ls == nil {
		return "", false
	}
	id, found := ls[LabelKubeVelaScheduledShardID]
	return id, found && id != ""
}

// SetScheduledShardID set shard-id to target object
func SetScheduledShardID(o client.Object, id string) {
	_ = k8s.AddLabel(o, LabelKubeVelaScheduledShardID, id)
}

// DelScheduledShardID delete shard-id from target object
func DelScheduledShardID(o client.Object) {
	_ = k8s.DeleteLabel(o, LabelKubeVelaScheduledShardID)
}

// PropagateScheduledShardIDLabel copy the shard-id from source obj to target
// obj, remove if not exist
func PropagateScheduledShardIDLabel(from client.Object, to client.Object) {
	if EnableSharding {
		sid, ok := GetScheduledShardID(from)
		if ok {
			SetScheduledShardID(to, sid)
		} else {
			DelScheduledShardID(to)
		}
	}
}

// IsMaster check if current instance is master
func IsMaster() bool {
	return ShardID == MasterShardID
}

// GetShardIDSuffix return suffix for shard id if enabled
func GetShardIDSuffix() string {
	if EnableSharding && !IsMaster() {
		return "-" + ShardID
	}
	return ""
}
