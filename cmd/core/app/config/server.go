/*
Copyright 2025 The KubeVela Authors.

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
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

// ServerConfig contains server-level configuration.
type ServerConfig struct {
	HealthAddr              string
	StorageDriver           string
	EnableLeaderElection    bool
	LeaderElectionNamespace string
	LeaseDuration           time.Duration
	RenewDeadline           time.Duration
	RetryPeriod             time.Duration
}

// NewServerConfig creates a new ServerConfig with defaults.
func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		HealthAddr:              ":9440",
		StorageDriver:           "Local",
		EnableLeaderElection:    false,
		LeaderElectionNamespace: "",
		LeaseDuration:           15 * time.Second,
		RenewDeadline:           10 * time.Second,
		RetryPeriod:             2 * time.Second,
	}
}

// AddFlags registers server configuration flags.
func (c *ServerConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.HealthAddr, "health-addr", c.HealthAddr,
		"The address the health endpoint binds to.")
	fs.StringVar(&c.StorageDriver, "storage-driver", c.StorageDriver,
		"Application storage driver.")
	fs.BoolVar(&c.EnableLeaderElection, "enable-leader-election", c.EnableLeaderElection,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&c.LeaderElectionNamespace, "leader-election-namespace", c.LeaderElectionNamespace,
		"Determines the namespace in which the leader election configmap will be created.")
	fs.DurationVar(&c.LeaseDuration, "leader-election-lease-duration", c.LeaseDuration,
		"The duration that non-leader candidates will wait to force acquire leadership")
	fs.DurationVar(&c.RenewDeadline, "leader-election-renew-deadline", c.RenewDeadline,
		"The duration that the acting controlplane will retry refreshing leadership before giving up")
	fs.DurationVar(&c.RetryPeriod, "leader-election-retry-period", c.RetryPeriod,
		"The duration the LeaderElector clients should wait between tries of actions")
}

// Validate checks if the server configuration is valid.
func (c *ServerConfig) Validate() error {
	if !c.EnableLeaderElection {
		return nil
	}
	if c.LeaseDuration <= 0 {
		return fmt.Errorf("leader-election-lease-duration must be greater than zero")
	}
	if c.RenewDeadline <= 0 {
		return fmt.Errorf("leader-election-renew-deadline must be greater than zero")
	}
	if c.RetryPeriod <= 0 {
		return fmt.Errorf("leader-election-retry-period must be greater than zero")
	}
	if c.LeaseDuration <= c.RenewDeadline {
		return fmt.Errorf("leader-election-lease-duration must be greater than leader-election-renew-deadline")
	}
	if c.RetryPeriod >= c.RenewDeadline-c.RenewDeadline/5 {
		return fmt.Errorf("leader-election-renew-deadline must be greater than leader-election-retry-period * Jitter (1.2)")
	}
	return nil
}
