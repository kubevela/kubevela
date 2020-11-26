package applicationconfiguration

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	v12 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllertest"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
)

func TestComponentHandler(t *testing.T) {
	q := controllertest.Queue{Interface: workqueue.New()}
	var curComp = &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
	}
	var createdRevisions = []appsv1.ControllerRevision{}
	var instance = ComponentHandler{
		Client: &test.MockClient{
			MockList: test.NewMockListFn(nil, func(obj runtime.Object) error {
				switch robj := obj.(type) {
				case *v1alpha2.ApplicationConfigurationList:
					lists := v1alpha2.ApplicationConfigurationList{
						Items: []v1alpha2.ApplicationConfiguration{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "app1",
								},
								Spec: v1alpha2.ApplicationConfigurationSpec{
									Components: []v1alpha2.ApplicationConfigurationComponent{{
										ComponentName: "comp1",
									}},
								},
							},
						},
					}
					lists.DeepCopyInto(robj)
					return nil
				case *appsv1.ControllerRevisionList:
					lists := appsv1.ControllerRevisionList{
						Items: createdRevisions,
					}
					lists.DeepCopyInto(robj)
				}
				return nil
			}),
			MockStatusUpdate: test.NewMockStatusUpdateFn(nil, func(obj runtime.Object) error {
				switch robj := obj.(type) {
				case *v1alpha2.Component:
					robj.DeepCopyInto(curComp)
				case *appsv1.ControllerRevision:
					for _, revision := range createdRevisions {
						if revision.Name == robj.Name {
							robj.DeepCopyInto(&revision)
						}
					}
				}
				return nil
			}),
			MockCreate: test.NewMockCreateFn(nil, func(obj runtime.Object) error {
				cur, ok := obj.(*appsv1.ControllerRevision)
				if ok {
					createdRevisions = append(createdRevisions, *cur)
				}
				return nil
			}),
			MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
				switch robj := obj.(type) {
				case *appsv1.ControllerRevision:
					if len(createdRevisions) == 0 {
						return nil
					}
					// test frame can't get the key, and just return the newest revision
					createdRevisions[len(createdRevisions)-1].DeepCopyInto(robj)
				case *v1alpha2.Component:
					robj.DeepCopyInto(curComp)
				}
				return nil
			}),
			MockDelete: test.NewMockDeleteFn(nil, func(obj runtime.Object) error {
				if robj, ok := obj.(*appsv1.ControllerRevision); ok {
					newRevisions := []appsv1.ControllerRevision{}
					for _, revision := range createdRevisions {
						if revision.Name == robj.Name {
							continue
						}

						newRevisions = append(newRevisions, revision)
					}
					createdRevisions = newRevisions
				}
				return nil
			}),
		},
		Logger:        logging.NewLogrLogger(ctrl.Log.WithName("test")),
		RevisionLimit: 2,
	}
	comp := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1", Generation: 1},
		Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v1"}}}}}}}},
	}

	// ============ Test Create Event Start ===================
	evt := event.CreateEvent{
		Object: comp,
		Meta:   comp.GetObjectMeta(),
	}
	instance.Create(evt, q)
	if q.Len() != 1 {
		t.Fatal("no event created, but suppose have one")
	}
	item, _ := q.Get()
	req := item.(reconcile.Request)
	// AppConfig event triggered, and compare revision created
	assert.Equal(t, req.Name, "app1")
	revisions := &appsv1.ControllerRevisionList{}
	err := instance.Client.List(context.TODO(), revisions)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(revisions.Items))
	assert.Equal(t, true, strings.HasPrefix(revisions.Items[0].Name, "comp1-"))
	gotComp := revisions.Items[0].Data.Object.(*v1alpha2.Component)
	// check component's spec saved in corresponding controllerRevision
	assert.Equal(t, comp.Spec, gotComp.Spec)
	// check component's status saved in corresponding controllerRevision
	assert.Equal(t, gotComp.Status.LatestRevision.Name, revisions.Items[0].Name)
	assert.Equal(t, gotComp.Status.LatestRevision.Revision, revisions.Items[0].Revision)
	// check component's status ObservedGeneration
	assert.Equal(t, gotComp.Status.ObservedGeneration, comp.Generation)
	q.Done(item)
	// ============ Test Create Event End ===================

	// ============ Test Update Event Start===================
	comp2 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1"},
		// change image
		Spec: v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v2"}}}}}}}},
	}
	curComp.Status.DeepCopyInto(&comp2.Status)
	updateEvt := event.UpdateEvent{
		ObjectOld: comp,
		MetaOld:   comp.GetObjectMeta(),
		ObjectNew: comp2,
		MetaNew:   comp2.GetObjectMeta(),
	}
	instance.Update(updateEvt, q)
	if q.Len() != 1 {
		t.Fatal("no event created, but suppose have one")
	}
	item, _ = q.Get()
	req = item.(reconcile.Request)
	// AppConfig event triggered, and compare revision created
	assert.Equal(t, req.Name, "app1")
	revisions = &appsv1.ControllerRevisionList{}
	err = instance.Client.List(context.TODO(), revisions)
	assert.NoError(t, err)
	// Component changed, we have two revision now.
	assert.Equal(t, 2, len(revisions.Items))
	for _, v := range revisions.Items {
		assert.Equal(t, true, strings.HasPrefix(v.Name, "comp1-"))
		if v.Revision == 2 {
			gotComp := v.Data.Object.(*v1alpha2.Component)
			// check component's spec saved in corresponding controllerRevision
			assert.Equal(t, comp2.Spec, gotComp.Spec)
			// check component's status saved in corresponding controllerRevision
			assert.Equal(t, gotComp.Status.LatestRevision.Name, v.Name)
			assert.Equal(t, gotComp.Status.LatestRevision.Revision, v.Revision)
		}
	}
	q.Done(item)
	// test no changes with component spec
	comp3 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1", Labels: map[string]string{"bar": "foo"}},
		Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v2"}}}}}}}},
	}
	curComp.Status.DeepCopyInto(&comp3.Status)
	updateEvt = event.UpdateEvent{
		ObjectOld: comp2,
		MetaOld:   comp2.GetObjectMeta(),
		ObjectNew: comp3,
		MetaNew:   comp3.GetObjectMeta(),
	}
	instance.Update(updateEvt, q)
	if q.Len() != 0 {
		t.Fatal("should not trigger event with nothing changed no change")
	}
	// ============ Test Update Event End ===================

	// ============ Test Revisions Start ===================
	// test clean revision
	comp4 := &v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{Namespace: "biz", Name: "comp1", Labels: map[string]string{"bar": "foo"}},
		Spec:       v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &v1.Deployment{Spec: v1.DeploymentSpec{Template: v12.PodTemplateSpec{Spec: v12.PodSpec{Containers: []v12.Container{{Image: "nginx:v3"}}}}}}}},
	}
	curComp.Status.DeepCopyInto(&comp4.Status)
	updateEvt = event.UpdateEvent{
		ObjectOld: comp2,
		MetaOld:   comp2.GetObjectMeta(),
		ObjectNew: comp4,
		MetaNew:   comp4.GetObjectMeta(),
	}
	instance.Update(updateEvt, q)
	revisions = &appsv1.ControllerRevisionList{}
	err = instance.Client.List(context.TODO(), revisions)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(revisions.Items), "Expected has two revisions")
	assert.Equal(t, "comp1", revisions.Items[0].Labels[ControllerRevisionComponentLabel],
		fmt.Sprintf("Expected revision has label %s: comp1", ControllerRevisionComponentLabel))
	// ============ Test Revisions End ===================
}

func TestConstructExtract(t *testing.T) {
	tests := []string{"tam1", "test-comp", "xx", "tt-x-x-c"}
	revisionNum := []int64{1, 5, 10, 100000}
	for idx, componentName := range tests {
		t.Run(fmt.Sprintf("tests %d for component[%s]", idx, componentName), func(t *testing.T) {
			revisionName := ConstructRevisionName(componentName, revisionNum[idx])
			got := ExtractComponentName(revisionName)
			if got != componentName {
				t.Errorf("want to get %s from %s but got %s", componentName, revisionName, got)
			}
		})
	}
}

func TestIsMatch(t *testing.T) {
	var appConfigs v1alpha2.ApplicationConfigurationList
	appConfigs.Items = []v1alpha2.ApplicationConfiguration{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "foo-app", Namespace: "foo-namespace"},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{ComponentName: "foo"}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bar-app", Namespace: "bar-namespace"},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{ComponentName: "bar"}},
			},
		},
	}
	got, namespaceNamed := isMatch(&appConfigs, "foo")
	assert.Equal(t, true, got)
	assert.Equal(t, types.NamespacedName{Name: "foo-app", Namespace: "foo-namespace"}, namespaceNamed)
	got, _ = isMatch(&appConfigs, "foo1")
	assert.Equal(t, false, got)
	got, namespaceNamed = isMatch(&appConfigs, "bar")
	assert.Equal(t, true, got)
	assert.Equal(t, types.NamespacedName{Name: "bar-app", Namespace: "bar-namespace"}, namespaceNamed)
	appConfigs.Items = nil
	got, _ = isMatch(&appConfigs, "foo")
	assert.Equal(t, false, got)
}

func TestSortedControllerRevision(t *testing.T) {
	appconfigs := []v1alpha2.ApplicationConfiguration{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "foo-app", Namespace: "test"},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{ComponentName: "foo", RevisionName: "revision1"}},
			},
		},
	}
	emptyAppconfigs := []v1alpha2.ApplicationConfiguration{}
	revision1 := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "revision1", Namespace: "foo-namespace"},
		Revision:   3,
	}
	revision2 := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "revision2", Namespace: "foo-namespace"},
		Revision:   1,
	}
	revision3 := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "revision2", Namespace: "foo-namespace"},
		Revision:   2,
	}
	revisions := []appsv1.ControllerRevision{
		revision1,
		revision2,
		revision3,
	}
	expectedRevison := []appsv1.ControllerRevision{
		revision2,
		revision3,
		revision1,
	}

	_, toKill, _ := sortedControllerRevision(appconfigs, revisions, 3)
	assert.Equal(t, 0, toKill, "Not over limit, needn't to delete")

	sortedRevisions, toKill, _ := sortedControllerRevision(emptyAppconfigs, revisions, 2)
	assert.Equal(t, expectedRevison, sortedRevisions, "Export controllerRevision sorted ascending accord to revision")
	assert.Equal(t, 1, toKill, "Over limit")

	_, toKill, liveHashes := sortedControllerRevision(appconfigs, revisions, 2)
	assert.Equal(t, 0, toKill, "Needn't to delete")
	assert.Equal(t, 1, len(liveHashes), "LiveHashes worked")
}
