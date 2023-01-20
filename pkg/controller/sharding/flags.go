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
	"github.com/spf13/pflag"
)

// AddFlags add sharding flags
func AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&EnableSharding, "enable-sharding", EnableSharding, "When sharding enabled, the controller will run as master (shard-id=master) or slave mode (shard-id is any non-empty string except master). Refer to https://github.com/kubevela/kubevela/blob/master/design/vela-core/sharding.md for details.")
	fs.StringVar(&ShardID, "shard-id", ShardID, "The id for sharding.")
	fs.StringSliceVar(&SchedulableShards, "schedulable-shards", SchedulableShards, "The shard ids that are available for scheduling. If empty, dynamic discovery will be used.")
	fs.DurationVar(&DynamicDiscoverySchedulerResyncPeriod, "sharding-slave-discovery-resync-period", DynamicDiscoverySchedulerResyncPeriod, "The resync period for default dynamic discovery scheduler.")
}
