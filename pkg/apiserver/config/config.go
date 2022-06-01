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

package config

import (
	"time"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
)

// Config config for server
type Config struct {
	// api server bind address
	BindAddr string
	// monitor metric path
	MetricPath string

	// Datastore config
	Datastore datastore.Config

	// LeaderConfig for leader election
	LeaderConfig leaderConfig

	// AddonCacheTime is how long between two cache operations
	AddonCacheTime time.Duration

	// DisableStatisticCronJob close the calculate system info cronJob
	DisableStatisticCronJob bool

	// KubeBurst the burst of kube client
	KubeBurst int

	// KubeQPS the QPS of kube client
	KubeQPS float64
}

type leaderConfig struct {
	ID       string
	LockName string
	Duration time.Duration
}
