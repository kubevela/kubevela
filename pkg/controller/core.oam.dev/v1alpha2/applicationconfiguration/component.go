package applicationconfiguration

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/pkg/logging"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	util "github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

// ControllerRevisionComponentLabel indicate which component the revision belong to
// This label is to filter revision by client api
const ControllerRevisionComponentLabel = "controller.oam.dev/component"

// ComponentHandler will watch component change and generate Revision automatically.
type ComponentHandler struct {
	Client        client.Client
	Logger        logging.Logger
	RevisionLimit int
}

// Create implements EventHandler
func (c *ComponentHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	if !c.createControllerRevision(evt.Meta, evt.Object) {
		// No revision created, return
		return
	}
	for _, req := range c.getRelatedAppConfig(evt.Meta) {
		q.Add(req)
	}
}

// Update implements EventHandler
func (c *ComponentHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	if !c.createControllerRevision(evt.MetaNew, evt.ObjectNew) {
		// No revision created, return
		return
	}
	// Note(wonderflow): MetaOld => MetaNew, requeue once is enough
	for _, req := range c.getRelatedAppConfig(evt.MetaNew) {
		q.Add(req)
	}
}

// Delete implements EventHandler
func (c *ComponentHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	// controllerRevision will be deleted by ownerReference mechanism
	// so we don't need to delete controllerRevision here.
	// but trigger an event to AppConfig controller, let it know.
	for _, req := range c.getRelatedAppConfig(evt.Meta) {
		q.Add(req)
	}
}

// Generic implements EventHandler
func (c *ComponentHandler) Generic(_ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
	// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
	// so we need to do nothing here.
}

func isMatch(appConfigs *v1alpha2.ApplicationConfigurationList, compName string) (bool, types.NamespacedName) {
	for _, app := range appConfigs.Items {
		for _, comp := range app.Spec.Components {
			if comp.ComponentName == compName {
				return true, types.NamespacedName{Namespace: app.Namespace, Name: app.Name}
			}
		}
	}
	return false, types.NamespacedName{}
}

func (c *ComponentHandler) getRelatedAppConfig(object metav1.Object) []reconcile.Request {
	var appConfigs v1alpha2.ApplicationConfigurationList
	err := c.Client.List(context.Background(), &appConfigs)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("error list all applicationConfigurations %v", err))
		return nil
	}
	var reqs []reconcile.Request
	if match, namespaceName := isMatch(&appConfigs, object.GetName()); match {
		reqs = append(reqs, reconcile.Request{NamespacedName: namespaceName})
	}
	return reqs
}

// IsRevisionDiff check whether there's any different between two component revision
func (c *ComponentHandler) IsRevisionDiff(mt metav1.Object, curComp *v1alpha2.Component) (bool, int64) {
	if curComp.Status.LatestRevision == nil {
		return true, 0
	}

	// client in controller-runtime will use infoermer cache
	// use client will be more efficient
	oldRev := &appsv1.ControllerRevision{}
	if err := c.Client.Get(context.TODO(), client.ObjectKey{Namespace: mt.GetNamespace(), Name: curComp.Status.LatestRevision.Name}, oldRev); err != nil {
		c.Logger.Info(fmt.Sprintf("get old controllerRevision %s error %v, will create new revision", curComp.Status.LatestRevision.Name, err), "componentName", mt.GetName())
		return true, curComp.Status.LatestRevision.Revision
	}
	if oldRev.Name == "" {
		c.Logger.Info(fmt.Sprintf("Not found controllerRevision %s", curComp.Status.LatestRevision.Name), "componentName", mt.GetName())
		return true, curComp.Status.LatestRevision.Revision
	}
	oldComp, err := util.UnpackRevisionData(oldRev)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("Unmarshal old controllerRevision %s error %v, will create new revision", curComp.Status.LatestRevision.Name, err), "componentName", mt.GetName())
		return true, oldRev.Revision
	}

	if reflect.DeepEqual(curComp.Spec, oldComp.Spec) {
		return false, oldRev.Revision
	}
	return true, oldRev.Revision
}

func newTrue() *bool {
	b := true
	return &b
}

func (c *ComponentHandler) createControllerRevision(mt metav1.Object, obj runtime.Object) bool {
	curComp := obj.(*v1alpha2.Component)
	comp := curComp.DeepCopy()
	diff, curRevision := c.IsRevisionDiff(mt, comp)
	if !diff {
		// No difference, no need to create new revision.
		return false
	}
	nextRevision := curRevision + 1
	revisionName := ConstructRevisionName(mt.GetName(), nextRevision)

	if comp.Status.ObservedGeneration != comp.Generation {
		comp.Status.ObservedGeneration = comp.Generation
	}

	comp.Status.LatestRevision = &v1alpha2.Revision{
		Name:     revisionName,
		Revision: nextRevision,
	}
	// set annotation to component
	revision := appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      revisionName,
			Namespace: comp.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1alpha2.SchemeGroupVersion.String(),
					Kind:       v1alpha2.ComponentKind,
					Name:       comp.Name,
					UID:        comp.UID,
					Controller: newTrue(),
				},
			},
			Labels: map[string]string{
				ControllerRevisionComponentLabel: comp.Name,
			},
		},
		Revision: nextRevision,
		Data:     runtime.RawExtension{Object: comp},
	}

	err := c.Client.Create(context.TODO(), &revision)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("error create controllerRevision %v", err), "componentName", mt.GetName())
		return false
	}

	err = c.Client.Status().Update(context.Background(), comp)
	if err != nil {
		c.Logger.Info(fmt.Sprintf("update component status latestRevision %s err %v", revisionName, err), "componentName", mt.GetName())
		return false
	}
	c.Logger.Info(fmt.Sprintf("ControllerRevision %s created", revisionName))
	if int64(c.RevisionLimit) < nextRevision {
		if err := c.cleanupControllerRevision(comp); err != nil {
			c.Logger.Info(fmt.Sprintf("failed to clean up revisions of Component %v.", err))
		}
	}
	return true
}

// get sorted controllerRevisions, prepare to delete controllerRevisions
func sortedControllerRevision(appConfigs []v1alpha2.ApplicationConfiguration, revisions []appsv1.ControllerRevision,
	revisionLimit int) (sortedRevisions []appsv1.ControllerRevision, toKill int, liveHashes map[string]bool) {
	liveHashes = make(map[string]bool)
	sortedRevisions = revisions

	// get all revisions used and skipped
	for _, appConfig := range appConfigs {
		for _, component := range appConfig.Spec.Components {
			if component.RevisionName != "" {
				liveHashes[component.RevisionName] = true
			}
		}
	}

	toKeep := revisionLimit + len(liveHashes)
	toKill = len(sortedRevisions) - toKeep
	if toKill <= 0 {
		toKill = 0
		return
	}
	// Clean up old revisions from smallest to highest revision (from oldest to newest)
	sort.Sort(historiesByRevision(sortedRevisions))

	return
}

// clean revisions when over limits
func (c *ComponentHandler) cleanupControllerRevision(curComp *v1alpha2.Component) error {
	labels := &metav1.LabelSelector{
		MatchLabels: map[string]string{
			ControllerRevisionComponentLabel: curComp.Name,
		},
	}
	selector, err := metav1.LabelSelectorAsSelector(labels)
	if err != nil {
		return err
	}

	// List and Get Object, controller-runtime will create Informer cache
	// and will get objects from cache
	revisions := &appsv1.ControllerRevisionList{}
	if err := c.Client.List(context.TODO(), revisions, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}

	// Get appConfigs and workloads filter controllerRevision used
	appConfigs := &v1alpha2.ApplicationConfigurationList{}
	if err := c.Client.List(context.Background(), appConfigs); err != nil {
		return err
	}

	// get sorted revisions
	controllerRevisions, toKill, liveHashes := sortedControllerRevision(appConfigs.Items, revisions.Items, c.RevisionLimit)
	for _, revision := range controllerRevisions {
		if toKill <= 0 {
			break
		}
		if hash := revision.GetName(); liveHashes[hash] {
			continue
		}
		// Clean up
		revisionToClean := revision
		if err := c.Client.Delete(context.TODO(), &revisionToClean); err != nil {
			return err
		}
		c.Logger.Info(fmt.Sprintf("ControllerRevision %s deleted", revision.Name))
		toKill--
	}
	return nil
}

// ConstructRevisionName will generate revisionName from componentName
// will be <componentName>-v<RevisionNumber>, for example: comp-v1
func ConstructRevisionName(componentName string, revision int64) string {
	return strings.Join([]string{componentName, fmt.Sprintf("v%d", revision)}, "-")
}

// ExtractComponentName will extract componentName from revisionName
func ExtractComponentName(revisionName string) string {
	splits := strings.Split(revisionName, "-")
	return strings.Join(splits[0:len(splits)-1], "-")
}

// historiesByRevision sort controllerRevision by revision
type historiesByRevision []appsv1.ControllerRevision

func (h historiesByRevision) Len() int      { return len(h) }
func (h historiesByRevision) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h historiesByRevision) Less(i, j int) bool {
	return h[i].Revision < h[j].Revision
}
