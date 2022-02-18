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

package util

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/oam-dev/kubevela/version"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	diskcached "k8s.io/client-go/discovery/cached/disk"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	ctrl "sigs.k8s.io/controller-runtime"
)

var defaultCacheDir = filepath.Join(homedir.HomeDir(), ".kube", "http-cache")

var _ genericclioptions.RESTClientGetter = &restConfigGetter{}

// NewRestConfigGetter create config for helm client.
// The helm client never thought it could be used inside a cluster so it
// took a dependency on the kube cli, we have to create a cli client getter from the rest.Config
func NewRestConfigGetter(namespace string) genericclioptions.RESTClientGetter {
	return &restConfigGetter{
		config:    ctrl.GetConfigOrDie(),
		namespace: namespace,
	}
}

// NewRestConfigGetterByConfig new rest config getter
func NewRestConfigGetterByConfig(config *rest.Config, namespace string) genericclioptions.RESTClientGetter {
	return &restConfigGetter{
		config:    config,
		namespace: namespace,
	}
}

type restConfigGetter struct {
	config    *rest.Config
	namespace string
}

func (r *restConfigGetter) ToRESTConfig() (*rest.Config, error) {
	return r.config, nil
}

// ToDiscoveryClient implements RESTClientGetter.
// Expects the AddFlags method to have been called.
// Returns a CachedDiscoveryInterface using a computed RESTConfig.
func (r *restConfigGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	// The more groups you have, the more discovery requests you need to make.
	// given 25 groups (our groups + a few custom resources) with one-ish version each, discovery needs to make 50 requests
	// double it just so we don't end up here again for a while.  This config is only used for discovery.
	config.Burst = 100

	// retrieve a user-provided value for the "cache-dir"
	// defaulting to ~/.kube/http-cache if no user-value is given.
	httpCacheDir := defaultCacheDir

	discoveryCacheDir := computeDiscoverCacheDir(filepath.Join(homedir.HomeDir(), ".kube", "cache", "discovery"), config.Host)
	return diskcached.NewCachedDiscoveryClientForConfig(config, discoveryCacheDir, httpCacheDir, 10*time.Minute)
}

// ToRESTMapper returns a mapper.
func (r *restConfigGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

func (r *restConfigGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// use the standard defaults for this client command
	// DEPRECATED: remove and replace with something more accurate
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}

	// bind auth info flag values to overrides
	if r.config.CertFile != "" {
		overrides.AuthInfo.ClientCertificate = r.config.CertFile
	}
	if r.config.KeyFile != "" {
		overrides.AuthInfo.ClientKey = r.config.KeyFile
	}
	if r.config.BearerToken != "" {
		overrides.AuthInfo.Token = r.config.BearerToken
	}

	overrides.AuthInfo.Impersonate = r.config.Impersonate.UserName
	overrides.AuthInfo.ImpersonateGroups = r.config.Impersonate.Groups
	overrides.AuthInfo.ImpersonateUserExtra = r.config.Impersonate.Extra

	if r.config.Username != "" {
		overrides.AuthInfo.Username = r.config.Username
	}
	if r.config.Password != "" {
		overrides.AuthInfo.Password = r.config.Password
	}

	// bind cluster flags
	if r.config.Host != "" {
		overrides.ClusterInfo.Server = r.config.Host
	}
	if r.config.CAFile != "" {
		overrides.ClusterInfo.CertificateAuthority = r.config.CAFile
	}
	overrides.ClusterInfo.InsecureSkipTLSVerify = r.config.Insecure

	if r.config.Timeout != 0 {
		overrides.Timeout = r.config.Timeout.String()
	}
	// set namespace
	overrides.Context.Namespace = r.namespace

	var clientConfig clientcmd.ClientConfig

	// we only have an interactive prompt when a password is allowed
	if r.config.Password == "" {
		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	} else {
		clientConfig = clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, overrides, os.Stdin)
	}

	return clientConfig
}

// overlyCautiousIllegalFileCharacters matches characters that *might* not be supported.  Windows is really restrictive, so this is really restrictive
var overlyCautiousIllegalFileCharacters = regexp.MustCompile(`[^(\w/\.)]`)

// computeDiscoverCacheDir takes the parentDir and the host and comes up with a "usually non-colliding" name.
func computeDiscoverCacheDir(parentDir, host string) string {
	// strip the optional scheme from host if its there:
	schemelessHost := strings.Replace(strings.Replace(host, "https://", "", 1), "http://", "", 1)
	// now do a simple collapse of non-AZ09 characters.  Collisions are possible but unlikely.  Even if we do collide the problem is short lived
	safeHost := overlyCautiousIllegalFileCharacters.ReplaceAllString(schemelessHost, "_")
	return filepath.Join(parentDir, safeHost)
}

// GenerateLeaderElectionID returns the Leader Election ID.
func GenerateLeaderElectionID(name string, versionedDeploy bool) string {
	return name + "-" + strings.ToLower(strings.ReplaceAll(version.VelaVersion, ".", "z"))
}
