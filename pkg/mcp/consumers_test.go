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

package mcp

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// consumerFixture is an Application with a database component that exports
// its endpoint into the Secret "petclinic-db", and a webservice that reads
// that Secret and is not ready.
type consumerFixture struct {
	dependsOn      bool
	exportWritten  bool
	instanceSynced string
	readsOwnSecret bool
}

// webDeployment is a Deployment with one unavailable replica whose pod
// template reads the Secret through an env value, an envFrom and a volume.
func webDeployment(secret string) *appsv1.Deployment {
	labels := map[string]string{oam.LabelAppName: "petclinic", oam.LabelAppNamespace: "team-a", oam.LabelAppComponent: "petclinic"}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "petclinic", Namespace: "team-a", UID: "petclinic-uid", Labels: labels, ResourceVersion: "9"},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "web", Image: "petclinic", Env: []corev1.EnvVar{
					{Name: "DB_HOST", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secret}, Key: "host"}}},
					{Name: "DB_PORT", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secret}, Key: "port"}}},
				}, EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-settings"}}}},
					VolumeMounts: []corev1.VolumeMount{{Name: "creds", MountPath: "/etc/creds", ReadOnly: true}}}},
				Volumes: []corev1.Volume{{Name: "creds", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secret}}}},
			}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 0, AvailableReplicas: 0, UnavailableReplicas: 1, ObservedGeneration: 1,
			Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse, Reason: "MinimumReplicasUnavailable", Message: "Deployment does not have minimum availability."}}},
	}
}

// crashLoopingPod is the Deployment's pod: running, not ready, with a
// container that was terminated before, which the resource tree reads as
// UnHealthy and the diagnosis derives onto the Deployment.
func crashLoopingPod() []client.Object {
	labels := map[string]string{oam.LabelAppName: "petclinic", oam.LabelAppNamespace: "team-a", oam.LabelAppComponent: "petclinic"}
	rsOwner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "petclinic", UID: "petclinic-uid", Controller: ptr.To(true)}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{Name: "petclinic-abc", Namespace: "team-a", UID: "petclinic-rs-uid", Labels: labels, OwnerReferences: []metav1.OwnerReference{rsOwner}},
		Spec:       appsv1.ReplicaSetSpec{Replicas: ptr.To[int32](1), Selector: &metav1.LabelSelector{MatchLabels: labels}},
		Status:     appsv1.ReplicaSetStatus{Replicas: 1},
	}
	podOwner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "petclinic-abc", UID: "petclinic-rs-uid", Controller: ptr.To(true)}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "petclinic-abc-xyz", Namespace: "team-a", UID: "petclinic-pod-uid", Labels: labels, OwnerReferences: []metav1.OwnerReference{podOwner}},
		Spec:       corev1.PodSpec{RestartPolicy: corev1.RestartPolicyAlways, Containers: []corev1.Container{{Name: "web", Image: "petclinic"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse}}, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "web", Ready: false, RestartCount: 5,
			State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1, Reason: "Error"}},
		}}},
	}
	return []client.Object{rs, pod}
}

func consumerObjects(f consumerFixture) []client.Object {
	web := oamcommon.ApplicationComponent{Name: "petclinic", Type: "webservice", Properties: raw(`{"image":"petclinic"}`)}
	if f.dependsOn {
		web.DependsOn = []string{"petclinic-db"}
	}
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "petclinic", Namespace: "team-a", Generation: 1},
		Spec: v1beta1.ApplicationSpec{Components: []oamcommon.ApplicationComponent{
			{Name: "petclinic-db", Type: "aurora-postgres", Properties: raw(`{"instanceClass":"db.serverless"}`)},
			web,
		}},
		Status: oamcommon.AppStatus{Phase: oamcommon.ApplicationRunningWorkflow, ObservedGeneration: 1,
			Services: []oamcommon.ApplicationComponentStatus{{Name: "petclinic-db", Healthy: true}, {Name: "petclinic", Healthy: false}}},
	}
	synced := "False"
	if f.exportWritten {
		synced = "True"
	}
	db := cloudResource("rds.services.k8s.aws/v1alpha1", "DBCluster", "petclinic-db", 1,
		[]map[string]any{{"type": "ACK.ResourceSynced", "status": "True"}}, nil)
	inst := cloudResource("rds.services.k8s.aws/v1alpha1", "DBInstance", "petclinic-db-instance-1", 1,
		[]map[string]any{{"type": "ACK.ResourceSynced", "status": f.instanceSynced}}, nil)
	fe := cloudResource("services.k8s.aws/v1alpha1", "FieldExport", "petclinic-db-conn-host", 1,
		[]map[string]any{{"type": "ACK.ResourceSynced", "status": synced}}, nil)
	_ = unstructured.SetNestedMap(fe.Object, map[string]any{
		"from": map[string]any{"path": ".status.endpoint", "resource": map[string]any{"group": "rds.services.k8s.aws", "kind": "DBCluster", "name": "petclinic-db"}},
		"to":   map[string]any{"kind": "secret", "name": "petclinic-db", "key": "host"},
	}, "spec")
	secretName := "petclinic-db"
	if f.readsOwnSecret {
		secretName = "petclinic-own"
	}
	deploy := webDeployment(secretName)

	rt := &v1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{Name: "petclinic-team-a", Labels: map[string]string{oam.LabelAppName: "petclinic", oam.LabelAppNamespace: "team-a"}},
		Spec:       v1beta1.ResourceTrackerSpec{Type: v1beta1.ResourceTrackerTypeVersioned, ApplicationGeneration: 1},
	}
	managed := func(component string, obj client.Object) v1beta1.ManagedResource {
		return v1beta1.ManagedResource{
			ClusterObjectReference: oamcommon.ClusterObjectReference{ObjectReference: corev1.ObjectReference{
				APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(), Kind: obj.GetObjectKind().GroupVersionKind().Kind,
				Namespace: obj.GetNamespace(), Name: obj.GetName(),
			}},
			OAMObjectReference: oamcommon.OAMObjectReference{Component: component},
		}
	}
	deploy.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
	rt.Spec.ManagedResources = []v1beta1.ManagedResource{managed("petclinic-db", db), managed("petclinic-db", inst), managed("petclinic-db", fe), managed("petclinic", deploy)}
	objs := append(append([]client.Object{}, platformFixtures()...), app, rt, db, inst, fe, deploy)
	objs = append(objs, crashLoopingPod()...)
	if f.readsOwnSecret {
		own := &corev1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: "petclinic-own", Namespace: "team-a"}}
		rt.Spec.ManagedResources = append(rt.Spec.ManagedResources, managed("petclinic", own))
		objs = append(objs, own)
	}
	return objs
}

var _ = Describe("diagnose_application consumers", func() {
	var h *harness

	AfterEach(func() {
		if h != nil {
			h.close()
			h = nil
		}
	})

	args := map[string]any{"name": "petclinic", "namespace": "team-a"}
	consumerFindings := func(out DiagnoseApplicationOutput) []Finding {
		var found []Finding
		for _, f := range out.Findings {
			// the tree's own "is NotReady" finding cites the same object; the
			// consumer finding is the one that says what it reads
			if f.Evidence == "Deployment team-a/petclinic in cluster local" && strings.Contains(f.Message, " reads ") {
				found = append(found, f)
			}
		}
		return found
	}

	It("blames the Application spec when a consumer reads an unwritten key without dependsOn", func() {
		h = startHarness(Options{}, consumerObjects(consumerFixture{exportWritten: false, instanceSynced: "False"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		found := consumerFindings(out)
		Expect(found).To(HaveLen(1), toJSON(out.Findings))
		Expect(found[0].Message).To(ContainSubstring(`component "petclinic": Deployment petclinic reads Secret "petclinic-db" key "host", which FieldExport petclinic-db-conn-host of component "petclinic-db" has not written yet`))
		Expect(found[0].Message).To(ContainSubstring(`"petclinic" declares no dependsOn on "petclinic-db", so the workflow applied both at once; the fix is in the Application spec`))
		Expect(found[0].Observed).To(ContainSubstring("resource tree health"))
		Expect(out.Evidence).To(ContainElement(ContainSubstring("Deployment team-a/petclinic in cluster local resourceVersion 9, read for the Secrets and ConfigMaps")))
	})

	It("blames the producer's definition when dependsOn is declared and the producer's member is still not ready", func() {
		h = startHarness(Options{}, consumerObjects(consumerFixture{dependsOn: true, exportWritten: true, instanceSynced: "False"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))

		found := consumerFindings(out)
		Expect(found).To(HaveLen(1), toJSON(out.Findings))
		Expect(found[0].Message).To(ContainSubstring(`reads Secret "petclinic-db" key "host", which component "petclinic-db" fills while its DBInstance petclinic-db-instance-1 is NotReady`))
		Expect(found[0].Message).To(ContainSubstring(`"petclinic" depends on "petclinic-db", so the workflow released it when "petclinic-db" reported healthy; that component's health rule does not cover everything a consumer needs, and the fix is in its definition`))
	})

	It("says nothing about the producer when it has done its part", func() {
		h = startHarness(Options{}, consumerObjects(consumerFixture{dependsOn: true, exportWritten: true, instanceSynced: "True"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(consumerFindings(out)).To(BeEmpty(), toJSON(out.Findings))
	})

	It("ignores a Secret the consumer's own component manages", func() {
		h = startHarness(Options{}, consumerObjects(consumerFixture{readsOwnSecret: true, instanceSynced: "False"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(consumerFindings(out)).To(BeEmpty(), toJSON(out.Findings))
	})

	It("blames the producer only when its state is known, never after a read-bound cutoff", func() {
		// exporter unsynced but the DBInstance could not be read: the
		// exporter's own verdict still says unwritten, so the finding stands
		// on that and never on the instance
		h = startHarness(Options{}, consumerObjects(consumerFixture{dependsOn: true, exportWritten: true, instanceSynced: "False"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(consumerFindings(out)).To(HaveLen(1))
		Expect(consumerFindings(out)[0].Message).To(ContainSubstring("is NotReady"))
	})

	It("skips the consumer read and says so when the diagnosis reached its live-read bound", func() {
		old := maxDiagnoseReads
		maxDiagnoseReads = 3 // the DBCluster, the DBInstance and the FieldExport use it up
		DeferCleanup(func() { maxDiagnoseReads = old })
		h = startHarness(Options{}, consumerObjects(consumerFixture{exportWritten: false, instanceSynced: "False"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", args, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(consumerFindings(out)).To(BeEmpty(), toJSON(out.Findings))
		Expect(out.Meta.Truncated).To(BeTrue())
		Expect(out.Unknowns).To(ContainElement("Deployment team-a/petclinic was not read for what it consumes: the diagnosis reached its bound of 3 live reads; narrow by component"))
	})

	It("skips the cross-component check and says so when narrowed to one component", func() {
		h = startHarness(Options{}, consumerObjects(consumerFixture{exportWritten: false, instanceSynced: "False"})...)
		var out DiagnoseApplicationOutput
		res := h.call("diagnose_application", map[string]any{"name": "petclinic", "namespace": "team-a", "component": "petclinic"}, &out)
		Expect(res.IsError).To(BeFalse(), toJSON(res))
		Expect(consumerFindings(out)).To(BeEmpty(), toJSON(out.Findings))
		Expect(out.Unknowns).To(ContainElement(ContainSubstring(`whether a workload of component "petclinic" waits on another component was not checked: the diagnosis was narrowed to that component`)))
	})

	It("extracts consumed references from a CronJob pod template and reports each reference once", func() {
		cj := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "batch/v1", "kind": "CronJob",
			"spec": map[string]any{"jobTemplate": map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
				"initContainers": []any{map[string]any{"name": "init", "envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "creds"}}}}},
				"containers": []any{map[string]any{"name": "job", "env": []any{
					map[string]any{"name": "A", "valueFrom": map[string]any{"configMapKeyRef": map[string]any{"name": "settings", "key": "a"}}},
					map[string]any{"name": "B", "valueFrom": map[string]any{"configMapKeyRef": map[string]any{"name": "settings", "key": "a"}}},
					map[string]any{"name": "C", "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": "maybe", "key": "c", "optional": true}}},
				}}},
				"volumes": []any{
					map[string]any{"name": "v", "configMap": map[string]any{"name": "settings"}},
					map[string]any{"name": "p", "projected": map[string]any{"sources": []any{
						map[string]any{"secret": map[string]any{"name": "tls"}},
						map[string]any{"configMap": map[string]any{"name": "ca", "optional": true}},
					}}},
				},
			}}}}},
		}}
		Expect(consumedRefs(cj)).To(Equal([]consumedRef{
			{kind: "Secret", name: "creds", via: viaEnv},
			{kind: "ConfigMap", name: "settings", key: "a", via: viaEnv},
			{kind: "Secret", name: "maybe", key: "c", via: viaEnv, optional: true},
			{kind: "ConfigMap", name: "settings", via: viaVolume},
			{kind: "Secret", name: "tls", via: viaVolume},
			{kind: "ConfigMap", name: "ca", via: viaVolume, optional: true},
		}))
	})
})

var _ = DescribeTable("consumedRefs finds the pod template of every workload kind",
	func(apiVersion, kind string, cronJob bool) {
		podSpec := map[string]any{"containers": []any{map[string]any{"name": "c", "envFrom": []any{map[string]any{"secretRef": map[string]any{"name": "creds"}}}}}}
		spec := map[string]any{"template": map[string]any{"spec": podSpec}}
		if cronJob {
			spec = map[string]any{"jobTemplate": map[string]any{"spec": spec}}
		}
		obj := &unstructured.Unstructured{Object: map[string]any{"apiVersion": apiVersion, "kind": kind, "spec": spec}}
		Expect(consumedRefs(obj)).To(Equal([]consumedRef{{kind: "Secret", name: "creds", via: viaEnv}}))
	},
	Entry("Deployment", "apps/v1", "Deployment", false),
	Entry("StatefulSet", "apps/v1", "StatefulSet", false),
	Entry("DaemonSet", "apps/v1", "DaemonSet", false),
	Entry("Job", "batch/v1", "Job", false),
	Entry("CronJob", "batch/v1", "CronJob", true),
)
