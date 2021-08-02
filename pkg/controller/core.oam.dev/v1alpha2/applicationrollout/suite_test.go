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

package applicationrollout

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	core_oam_dev "github.com/oam-dev/kubevela/apis/core.oam.dev"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var recorder = NewFakeRecorder(10000)
var k8sClient client.Client
var testEnv *envtest.Environment
var reconciler *Reconciler
var testScheme = runtime.NewScheme()

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	By("bootstrapping test environment")
	var yamlPath string
	if _, set := os.LookupEnv("COMPATIBILITY_TEST"); set {
		yamlPath = "../../../../../test/compatibility-test/testdata"
	} else {
		yamlPath = filepath.Join("../../../../..", "charts", "vela-core", "crds")
	}
	logf.Log.Info("start application rollout suit test", "yaml_path", yamlPath)
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			yamlPath, // this has all the required CRDs,
			filepath.Join("..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = core_oam_dev.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	dm, err := discoverymapper.New(cfg)
	Expect(err).To(BeNil())

	reconciler = &Reconciler{
		Client:               k8sClient,
		Scheme:               scheme.Scheme,
		dm:                   dm,
		Recorder:             event.NewAPIRecorder(recorder),
		concurrentReconciles: 1,
	}

	// TODO write test here

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

type FakeRecorder struct {
	Events  chan string
	Message map[string][]*Events
}

type Events struct {
	Name      string
	Namespace string
	EventType string
	Reason    string
	Message   string
}

func (f *FakeRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	if f.Events != nil {
		objectMeta, err := meta.Accessor(object)
		if err != nil {
			return
		}

		event := &Events{
			Name:      objectMeta.GetName(),
			Namespace: objectMeta.GetNamespace(),
			EventType: eventtype,
			Reason:    reason,
			Message:   message,
		}

		records, ok := f.Message[objectMeta.GetName()]
		if !ok {
			f.Message[objectMeta.GetName()] = []*Events{event}
			return
		}

		records = append(records, event)
		f.Message[objectMeta.GetName()] = records

	}
}

func (f *FakeRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Event(object, eventtype, reason, messageFmt)
}

func (f *FakeRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Eventf(object, eventtype, reason, messageFmt, args...)
}

func (f *FakeRecorder) GetEventsWithName(name string) ([]*Events, error) {
	records, ok := f.Message[name]
	if !ok {
		return nil, errors.New("not found events")
	}

	return records, nil
}

// NewFakeRecorder creates new fake event recorder with event channel with
// buffer of given size.
func NewFakeRecorder(bufferSize int) *FakeRecorder {
	return &FakeRecorder{
		Events:  make(chan string, bufferSize),
		Message: make(map[string][]*Events),
	}
}

// randomNamespaceName generates a random name based on the basic name.
// Running each ginkgo case in a new namespace with a random name can avoid
// waiting a long time to GC namesapce.
func randomNamespaceName(basic string) string {
	return fmt.Sprintf("%s-%s", basic, strconv.FormatInt(rand.Int63(), 16))
}
