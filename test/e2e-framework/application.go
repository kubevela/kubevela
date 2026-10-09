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

package framework

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func (f *Framework) CreateNamespace(ctx context.Context, namespaceName string) corev1.Namespace {
	ns, err := CreateFreshNamespace(ctx, f.Client, namespaceName)
	gomega.Expect(err).To(gomega.Succeed())
	return ns
}

func CreateFreshNamespace(ctx context.Context, cli client.Client, name string) (corev1.Namespace, error) {
	var token [12]byte
	if _, err := cryptorand.Read(token[:]); err != nil {
		return corev1.Namespace{}, err
	}
	const ownerLabel = "e2e.kubevela.io/namespace-owner"
	owner := hex.EncodeToString(token[:])
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{ownerLabel: owner}}}
	var lastErr error
	hadTransientFailure := false
	err := wait.PollUntilContextTimeout(ctx, 300*time.Millisecond, 3*time.Second, true, func(pollCtx context.Context) (bool, error) {
		attempt := ns.DeepCopy()
		lastErr = cli.Create(pollCtx, attempt)
		if lastErr == nil {
			ns = *attempt
			return true, nil
		}
		if apierrors.IsAlreadyExists(lastErr) && !hadTransientFailure {
			return false, lastErr
		}
		if !apierrors.IsAlreadyExists(lastErr) && !transientNamespaceAPIError(lastErr) {
			return false, lastErr
		}
		hadTransientFailure = true
		// A timeout may lose the response after the API server accepted the
		// create. Recover only a namespace carrying this attempt's token.
		var existing corev1.Namespace
		getErr := cli.Get(pollCtx, client.ObjectKey{Name: name}, &existing)
		if getErr == nil {
			if existing.Labels[ownerLabel] != owner {
				return false, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "namespaces"}, name)
			}
			ns = existing
			return true, nil
		}
		if !apierrors.IsNotFound(getErr) && !transientNamespaceAPIError(getErr) {
			return false, getErr
		}
		return false, nil
	})
	if err != nil {
		return corev1.Namespace{}, errors.Join(err, lastErr)
	}
	return ns, nil
}

func transientNamespaceAPIError(err error) bool {
	var networkErr net.Error
	return apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) || apierrors.IsTooManyRequests(err) ||
		apierrors.IsInternalError(err) || errors.Is(err, io.EOF) ||
		utilnet.IsConnectionRefused(err) || utilnet.IsConnectionReset(err) ||
		(errors.As(err, &networkErr) && networkErr.Timeout())
}

func (f *Framework) CreateServiceAccount(ctx context.Context, ns, name string) {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
	gomega.Eventually(
		func() error {
			return f.Client.Create(ctx, &sa)
		},
		time.Second*3, time.Millisecond*300).Should(gomega.SatisfyAny(gomega.BeNil(), &util.AlreadyExistMatcher{}))
}

func (f *Framework) ApplyApp(ctx context.Context, namespaceName, source string, app *v1beta1.Application) {
	ginkgo.By("Apply an application")
	var newApp v1beta1.Application
	gomega.Expect(common.ReadYamlToObject(TestDataPath("app", source), &newApp)).Should(gomega.BeNil())
	newApp.Namespace = namespaceName
	gomega.Eventually(func() error {
		return f.Client.Create(ctx, newApp.DeepCopy())
	}, 10*time.Second, 500*time.Millisecond).Should(gomega.Succeed())
	ginkgo.By("Get Application latest status")
	gomega.Eventually(
		func() *oamcomm.Revision {
			_ = f.Client.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: newApp.Name}, app)
			if app.Status.LatestRevision != nil {
				return app.Status.LatestRevision
			}
			return nil
		},
		time.Second*30, time.Millisecond*500).ShouldNot(gomega.BeNil())
}

func (f *Framework) UpdateApp(ctx context.Context, namespaceName, target string, app *v1beta1.Application) {
	ginkgo.By("Update the application to target spec during rolling")
	var targetApp v1beta1.Application
	gomega.Expect(common.ReadYamlToObject(TestDataPath("app", target), &targetApp)).Should(gomega.BeNil())
	gomega.Eventually(
		func() error {
			_ = f.Client.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app.Name}, app)
			app.Spec = targetApp.Spec
			return f.Client.Update(ctx, app)
		}, time.Second*5, time.Millisecond*500).Should(gomega.Succeed())
}

func (f *Framework) VerifyApplicationPhase(ctx context.Context, ns, appName string, expected oamcomm.ApplicationPhase) {
	var testApp v1beta1.Application
	gomega.Eventually(func() error {
		err := f.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: appName}, &testApp)
		if err != nil {
			return err
		}
		if testApp.Status.Phase != expected {
			return fmt.Errorf("application status wants %s, actually %s", expected, testApp.Status.Phase)
		}
		return nil
	}, 120*time.Second, time.Second).Should(gomega.BeNil())
}

func (f *Framework) VerifyApplicationDelaySuspendExpected(ctx context.Context, ns, appName, suspendStep, nextStep, duration string) {
	var testApp v1beta1.Application
	gomega.Eventually(func() error {
		waitDuration, err := time.ParseDuration(duration)
		if err != nil {
			return err
		}

		err = f.Client.Get(ctx, client.ObjectKey{Namespace: ns, Name: appName}, &testApp)
		if err != nil {
			return err
		}

		if testApp.Status.Workflow == nil {
			return fmt.Errorf("application wait to start workflow")
		}

		if testApp.Status.Workflow.Finished {
			var suspendStartTime, nextStepStartTime metav1.Time
			var sFlag, nFlag bool

			for _, wfStatus := range testApp.Status.Workflow.Steps {
				if wfStatus.Name == suspendStep {
					suspendStartTime = wfStatus.FirstExecuteTime
					sFlag = true
					continue
				}

				if wfStatus.Name == nextStep {
					nextStepStartTime = wfStatus.FirstExecuteTime
					nFlag = true
				}
			}

			if !sFlag {
				return fmt.Errorf("application can not find suspend step: %s", suspendStep)
			}

			if !nFlag {
				return fmt.Errorf("application can not find next step: %s", nextStep)
			}

			dd := nextStepStartTime.Sub(suspendStartTime.Time)
			if waitDuration > dd {
				return fmt.Errorf("application suspend wait duration wants more than %s, actually %s", duration, dd.String())
			}

			return nil
		}
		return fmt.Errorf("application status workflow finished wants true, actually false")
	}, 120*time.Second, time.Second).Should(gomega.BeNil())
}

func (f *Framework) VerifyWorkloadRunningExpected(ctx context.Context, namespaceName, workloadName string, replicas int32, image string) {
	var workload v1.Deployment
	ginkgo.By("Verify Workload running as expected")
	gomega.Eventually(
		func() error {
			if err := f.Client.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: workloadName}, &workload); err != nil {
				return err
			}
			if workload.Status.ReadyReplicas != replicas {
				return fmt.Errorf("expect replicas %v != real %v", replicas, workload.Status.ReadyReplicas)
			}
			if workload.Spec.Template.Spec.Containers[0].Image != image {
				return fmt.Errorf("expect replicas %v != real %v", image, workload.Spec.Template.Spec.Containers[0].Image)
			}
			return nil
		},
		time.Second*60, time.Millisecond*500).Should(gomega.BeNil())
}
