/*
Copyright 2026 The KubeVela Authors.

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

package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"
	"github.com/pkg/errors"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/cmd/core/app/options"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/servingcert"
	webhookcerts "github.com/oam-dev/kubevela/pkg/webhook/certs"
)

// The serving certs that vela-core manages: the cluster-gateway APIService
// cert and the admission webhook cert. They replace the chart's
// kube-webhook-certgen hook Jobs.

var (
	// servingCertResyncPeriod is the interval between checks of the serving
	// certs that vela-core manages. Each check renews a cert if necessary. It
	// also repairs a caBundle or failurePolicy that something changed, for
	// example a GitOps sync.
	servingCertResyncPeriod = time.Minute
	// startupRetryInterval and startupRetryTimeout control how long the first
	// setup waits for objects that the chart creates with the Deployment. These
	// objects include the APIService and the webhook configurations.
	startupRetryInterval = 2 * time.Second
	startupRetryTimeout  = 2 * time.Minute
	// restartJitter spreads the restarts of several replicas after a CA change.
	restartJitter = 30 * time.Second
)

// resyncJitterFactor spreads the resyncs of several replicas over 10% of the
// period, so that they do not all write at the same time.
const resyncJitterFactor = 0.1

// retryStartup runs fn until fn succeeds or startupRetryTimeout ends. It gives
// fn a context that ends at the same time, so a request that hangs does not
// keep startup waiting. It returns the last error from fn.
func retryStartup(ctx context.Context, name string, fn func(context.Context) error) error {
	var lastErr error
	err := wait.PollImmediateWithContext(ctx, startupRetryInterval, startupRetryTimeout, func(ctx context.Context) (bool, error) {
		lastErr = fn(ctx)
		if lastErr != nil {
			klog.InfoS("Waiting to set up serving cert", "for", name, "reason", lastErr.Error())
		}
		// A failed attempt is retried, not returned: the poll ends only on
		// success or at the timeout.
		return lastErr == nil, nil
	})
	if lastErr != nil {
		return lastErr
	}
	return err
}

// startClusterGatewayTLS sets up the cluster-gateway serving cert in the Secret
// secretRef, for the Service serviceRef if it is not empty. Then it keeps the
// cert valid in the background. In direct mode
// (gateway.URL is set), it sets gateway.CAFile to the file that holds the CA.
// It sets gateway.CAFile once, before a client reads the options.
func startClusterGatewayTLS(ctx context.Context, c ctrlclient.Client, secretRef, serviceRef string,
	gateway *pkgmulticluster.ClusterGatewayClientOptions, restart func()) error {
	namespace, name, err := parseNamespacedName("cluster-gateway-tls-secret", secretRef)
	if err != nil {
		return err
	}
	var svc *servingcert.Service
	if serviceRef != "" {
		svcNamespace, svcName, err := parseNamespacedName("cluster-gateway-service", serviceRef)
		if err != nil {
			return err
		}
		svc = &servingcert.Service{Namespace: svcNamespace, Name: svcName}
	}
	caFile := ""
	if gateway.URL != "" {
		caFile = gateway.CAFile
		if caFile == "" {
			caFile = filepath.Join(os.TempDir(), "cluster-gateway", servingcert.CAKey)
		}
		gateway.CAFile = caFile
	}
	var trusted []byte
	if err = retryStartup(ctx, "cluster-gateway TLS", func(ctx context.Context) (err error) {
		trusted, err = setupClusterGatewayTLS(ctx, c, namespace, name, svc, caFile)
		return err
	}); err != nil {
		return err
	}
	go wait.JitterUntilWithContext(ctx, clusterGatewayTLSResync(c, namespace, name, svc, caFile, trusted, restart),
		servingCertResyncPeriod, resyncJitterFactor, true)
	return nil
}

// setupClusterGatewayTLS makes sure that the cluster-gateway serving cert in
// the Secret <namespace>/<name> is valid. It also makes sure that the
// cluster-gateway APIService trusts the CA. It returns the CA bundle. If
// caFile is set (direct mode), it also writes the bundle to caFile. vela-core
// cannot mount a Secret that it creates itself.
func setupClusterGatewayTLS(ctx context.Context, c ctrlclient.Client, namespace, name string, svc *servingcert.Service,
	caFile string) ([]byte, error) {
	bundle, err := multicluster.EnsureClusterGatewayTLS(ctx, c, namespace, name, svc)
	if err != nil {
		return nil, err
	}
	if caFile == "" {
		return bundle, nil
	}
	changed, err := servingcert.WriteFileIfChanged(caFile, bundle)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write cluster-gateway CA file")
	}
	if changed {
		klog.InfoS("Wrote the cluster-gateway CA for direct mode", "file", caFile)
	}
	return bundle, nil
}

// clusterGatewayTLSResync returns the periodic check for the cluster-gateway
// cert. The direct-mode client reads its CA only once, when vela-core builds
// the client. Thus, if the CA set changes, vela-core restarts to load it.
// A serving cert renewal with the same CA is the usual case. It does not need
// a restart.
func clusterGatewayTLSResync(c ctrlclient.Client, namespace, name string, svc *servingcert.Service, caFile string,
	trusted []byte, restart func()) func(context.Context) {
	return func(ctx context.Context) {
		bundle, err := setupClusterGatewayTLS(ctx, c, namespace, name, svc, caFile)
		if err != nil {
			klog.ErrorS(err, "Failed to resync the cluster-gateway serving cert")
			return
		}
		// Restart when the CA set changes in either direction: a new CA must be
		// trusted, and a CA dropped after the overlap must stop being trusted.
		if caFile != "" && (!servingcert.Covers(trusted, bundle) || !servingcert.Covers(bundle, trusted)) {
			restart()
		}
	}
}

// restartForNewCA returns a func that stops vela-core gracefully after a
// random delay of up to restartJitter. Because of the delay, replicas do not
// all restart at the same time. Then the restart policy of the container
// starts vela-core again.
func restartForNewCA(stop context.CancelFunc) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			delay := time.Duration(utilrand.Int63nRange(0, int64(restartJitter)+1))
			klog.InfoS("The cluster-gateway CA was replaced, restarting so the direct-mode client trusts it", "in", delay)
			time.AfterFunc(delay, stop)
		})
	}
}

// webhookTLSOptions checks the webhook TLS flags and converts them into the
// options for webhookcerts.Ensure.
func webhookTLSOptions(s *options.CoreOptions) (webhookcerts.Options, error) {
	namespace, name, err := parseNamespacedName("webhook-tls-secret", s.WebhookTLSSecret)
	if err != nil {
		return webhookcerts.Options{}, err
	}
	if s.WebhookConfigurationName == "" {
		return webhookcerts.Options{}, fmt.Errorf("--webhook-configuration is required with --webhook-tls-secret")
	}
	policy := admissionregistrationv1.FailurePolicyType(s.WebhookFailurePolicy)
	if policy != "" && policy != admissionregistrationv1.Fail && policy != admissionregistrationv1.Ignore {
		return webhookcerts.Options{}, fmt.Errorf("invalid --webhook-failure-policy %q, want Fail or Ignore", s.WebhookFailurePolicy)
	}
	return webhookcerts.Options{
		SecretNamespace:   namespace,
		SecretName:        name,
		ConfigurationName: s.WebhookConfigurationName,
		FailurePolicy:     policy,
		ConversionCRDs:    s.WebhookConversionCRDs,
	}, nil
}

// ensureWebhookTLS makes sure that the webhook serving cert is valid. It
// writes the cert to certDir for the webhook server and sets the caBundle of
// the webhook configurations. If applyPolicy is true, it also sets
// opts.FailurePolicy. Set applyPolicy only when the webhook server serves
// requests. The webhook server reloads the files when they change.
func ensureWebhookTLS(ctx context.Context, c ctrlclient.Client, opts webhookcerts.Options, certDir string, applyPolicy bool) error {
	if !applyPolicy {
		opts.FailurePolicy = ""
	}
	res, err := webhookcerts.Ensure(ctx, c, opts)
	if err != nil {
		return err
	}
	changed, err := webhookcerts.WriteFiles(certDir, res)
	if err != nil {
		return err
	}
	if changed {
		klog.InfoS("Wrote the webhook serving cert", "secret", opts.SecretNamespace+"/"+opts.SecretName, "certDir", certDir)
	}
	return nil
}

// resyncWebhookTLS keeps the webhook serving cert valid until ctx is done.
// Until serving reports that the webhook server is up, it checks every
// startupRetryInterval and does not change the failurePolicy. After it sets
// the failurePolicy, it checks every servingCertResyncPeriod.
func resyncWebhookTLS(ctx context.Context, c ctrlclient.Client, opts webhookcerts.Options, certDir string, serving func() bool) {
	policySet := false
	for {
		period := servingCertResyncPeriod
		if !policySet {
			period = startupRetryInterval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(period):
		}
		up := serving()
		if err := ensureWebhookTLS(ctx, c, opts, certDir, up); err != nil {
			klog.ErrorS(err, "Failed to resync the webhook serving cert")
			continue
		}
		if up && !policySet {
			policySet = true
			klog.InfoS("Webhook server is serving, set the webhook failurePolicy", "failurePolicy", opts.FailurePolicy)
		}
	}
}

// parseNamespacedName divides the <namespace>/<name> value of flag into its two
// parts.
func parseNamespacedName(flag, ref string) (namespace, name string, err error) {
	namespace, name, ok := strings.Cut(ref, "/")
	if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("invalid --%s %q, want <namespace>/<name>", flag, ref)
	}
	return namespace, name, nil
}
