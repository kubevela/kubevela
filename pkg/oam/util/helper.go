package util

import (
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"reflect"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	cpv1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
)

var (
	// KindDeployment is the k8s Deployment kind.
	KindDeployment = reflect.TypeOf(appsv1.Deployment{}).Name()
	// KindService is the k8s Service kind.
	KindService = reflect.TypeOf(corev1.Service{}).Name()
	// ReconcileWaitResult is the time to wait between reconciliation.
	ReconcileWaitResult = reconcile.Result{RequeueAfter: 30 * time.Second}
)

const (
	// TraitPrefixKey is prefix of trait name
	TraitPrefixKey = "trait"

	// Dummy used for dummy definition
	Dummy = "dummy"

	// DummyTraitMessage is a message for trait which don't have definition found
	DummyTraitMessage = "No valid TraitDefinition found, all framework capabilities will work as default or disabled"

	// DefinitionNamespaceEnv is env key for specifying a namespace to fetch definition
	DefinitionNamespaceEnv = "DEFINITION_NAMESPACE"
)

const (
	// ErrUpdateStatus is the error while applying status.
	ErrUpdateStatus = "cannot apply status"
	// ErrLocateAppConfig is the error while locating parent application.
	ErrLocateAppConfig = "cannot locate the parent application configuration to emit event to"
	// ErrLocateWorkload is the error while locate the workload
	ErrLocateWorkload = "cannot find the workload that the trait is referencing to"
	// ErrFetchChildResources is the error while fetching workload child resources
	ErrFetchChildResources = "failed to fetch workload child resources"

	errFmtGetComponentRevision   = "cannot get component revision %q"
	errFmtControllerRevisionData = "cannot get valid component data from controllerRevision %q"
	errFmtGetComponent           = "cannot get component %q"
	errFmtInvalidRevisionType    = "invalid type of revision %s, type should not be %v"
)

// A ConditionedObject is an Object type with condition field
type ConditionedObject interface {
	oam.Object

	oam.Conditioned
}

// LocateParentAppConfig locate the parent application configuration object
func LocateParentAppConfig(ctx context.Context, client client.Client, oamObject oam.Object) (oam.Object, error) {
	var acName string
	var eventObj = &v1alpha2.ApplicationConfiguration{}
	// locate the appConf name from the owner list
	for _, o := range oamObject.GetOwnerReferences() {
		if o.Kind == v1alpha2.ApplicationConfigurationKind {
			acName = o.Name
			break
		}
	}
	if len(acName) > 0 {
		nn := types.NamespacedName{
			Name:      acName,
			Namespace: oamObject.GetNamespace(),
		}
		if err := client.Get(ctx, nn, eventObj); err != nil {
			return nil, err
		}
		return eventObj, nil
	}
	return nil, errors.Errorf(ErrLocateAppConfig)
}

// FetchWorkload fetch the workload that a trait refers to
func FetchWorkload(ctx context.Context, c client.Client, mLog logr.Logger, oamTrait oam.Trait) (
	*unstructured.Unstructured, error) {
	var workload unstructured.Unstructured
	workloadRef := oamTrait.GetWorkloadReference()
	if len(workloadRef.Kind) == 0 || len(workloadRef.APIVersion) == 0 || len(workloadRef.Name) == 0 {
		err := errors.New("no workload reference")
		mLog.Error(err, ErrLocateWorkload)
		return nil, err
	}
	workload.SetAPIVersion(workloadRef.APIVersion)
	workload.SetKind(workloadRef.Kind)
	wn := client.ObjectKey{Name: workloadRef.Name, Namespace: oamTrait.GetNamespace()}
	if err := c.Get(ctx, wn, &workload); err != nil {
		mLog.Error(err, "Workload not find", "kind", workloadRef.Kind, "workload name", workloadRef.Name)
		return nil, err
	}
	mLog.Info("Get the workload the trait is pointing to", "workload name", workload.GetName(),
		"workload APIVersion", workload.GetAPIVersion(), "workload Kind", workload.GetKind(), "workload UID",
		workload.GetUID())
	return &workload, nil
}

// GetDummyTraitDefinition will generate a dummy TraitDefinition for CustomResource that won't block app from running.
// OAM runtime will report warning if they got this dummy definition.
func GetDummyTraitDefinition(u *unstructured.Unstructured) *v1alpha2.TraitDefinition {
	return &v1alpha2.TraitDefinition{
		TypeMeta: metav1.TypeMeta{Kind: v1alpha2.TraitDefinitionKind, APIVersion: v1alpha2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: Dummy, Annotations: map[string]string{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"name":       u.GetName(),
		}},
		Spec: v1alpha2.TraitDefinitionSpec{Reference: v1alpha2.DefinitionReference{Name: Dummy}},
	}
}

// GetDummyWorkloadDefinition will generate a dummy WorkloadDefinition for CustomResource that won't block app from running.
// OAM runtime will report warning if they got this dummy definition.
func GetDummyWorkloadDefinition(u *unstructured.Unstructured) *v1alpha2.WorkloadDefinition {
	return &v1alpha2.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{Kind: v1alpha2.WorkloadDefinitionKind, APIVersion: v1alpha2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: Dummy, Annotations: map[string]string{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"name":       u.GetName(),
		}},
		Spec: v1alpha2.WorkloadDefinitionSpec{Reference: v1alpha2.DefinitionReference{Name: Dummy}},
	}
}

// FetchScopeDefinition fetch corresponding scopeDefinition given a scope
func FetchScopeDefinition(ctx context.Context, r client.Reader, dm discoverymapper.DiscoveryMapper,
	scope *unstructured.Unstructured) (*v1alpha2.ScopeDefinition, error) {
	// The name of the scopeDefinition CR is the CRD name of the scope
	// TODO(wonderflow): we haven't support scope definition label type yet.
	spName, err := GetDefinitionName(dm, scope, "")
	if err != nil {
		return nil, err
	}
	nn := GenNamespacedDefinitionName(spName)
	// Fetch the corresponding scopeDefinition CR
	scopeDefinition := &v1alpha2.ScopeDefinition{}
	if err := r.Get(ctx, nn, scopeDefinition); err != nil {
		return nil, err
	}
	return scopeDefinition, nil
}

// FetchTraitDefinition fetch corresponding traitDefinition given a trait
func FetchTraitDefinition(ctx context.Context, r client.Reader, dm discoverymapper.DiscoveryMapper,
	trait *unstructured.Unstructured) (*v1alpha2.TraitDefinition, error) {
	// The name of the traitDefinition CR is the CRD name of the trait
	trName, err := GetDefinitionName(dm, trait, oam.TraitTypeLabel)
	if err != nil {
		return nil, err
	}
	nn := GenNamespacedDefinitionName(trName)
	// Fetch the corresponding traitDefinition CR
	traitDefinition := &v1alpha2.TraitDefinition{}
	if err := r.Get(ctx, nn, traitDefinition); err != nil {
		return nil, err
	}
	return traitDefinition, nil
}

// FetchWorkloadDefinition fetch corresponding workloadDefinition given a workload
func FetchWorkloadDefinition(ctx context.Context, r client.Reader, dm discoverymapper.DiscoveryMapper,
	workload *unstructured.Unstructured) (*v1alpha2.WorkloadDefinition, error) {
	// The name of the workloadDefinition CR is the CRD name of the component
	wldName, err := GetDefinitionName(dm, workload, oam.WorkloadTypeLabel)
	if err != nil {
		return nil, err
	}
	nn := GenNamespacedDefinitionName(wldName)
	// Fetch the corresponding workloadDefinition CR
	workloadDefinition := &v1alpha2.WorkloadDefinition{}
	if err := r.Get(ctx, nn, workloadDefinition); err != nil {
		return nil, err
	}
	return workloadDefinition, nil
}

// GenNamespacedDefinitionName generate definition name with customized namespace
func GenNamespacedDefinitionName(dn string) types.NamespacedName {
	if dns := os.Getenv(DefinitionNamespaceEnv); dns != "" {
		return types.NamespacedName{Name: dn, Namespace: dns}
	}
	return types.NamespacedName{Name: dn}
}

// FetchWorkloadChildResources fetch corresponding child resources given a workload
func FetchWorkloadChildResources(ctx context.Context, mLog logr.Logger, r client.Reader,
	dm discoverymapper.DiscoveryMapper, workload *unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	// Fetch the corresponding workloadDefinition CR
	workloadDefinition, err := FetchWorkloadDefinition(ctx, r, dm, workload)
	if err != nil {
		// No definition will won't block app from running
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return fetchChildResources(ctx, mLog, r, workload, workloadDefinition.Spec.ChildResourceKinds)
}

func fetchChildResources(ctx context.Context, mLog logr.Logger, r client.Reader, workload *unstructured.Unstructured,
	wcrl []v1alpha2.ChildResourceKind) ([]*unstructured.Unstructured, error) {
	var childResources []*unstructured.Unstructured
	// list by each child resource type with namespace and possible label selector
	for _, wcr := range wcrl {
		crs := unstructured.UnstructuredList{}
		crs.SetAPIVersion(wcr.APIVersion)
		crs.SetKind(wcr.Kind)
		mLog.Info("List child resource kind", "APIVersion", wcr.APIVersion, "Kind", wcr.Kind, "owner UID",
			workload.GetUID())
		if err := r.List(ctx, &crs, client.InNamespace(workload.GetNamespace()),
			client.MatchingLabels(wcr.Selector)); err != nil {
			mLog.Error(err, "failed to list object", "api version", crs.GetAPIVersion(), "kind", crs.GetKind())
			return nil, err
		}
		// pick the ones that is owned by the workload
		for _, cr := range crs.Items {
			for _, owner := range cr.GetOwnerReferences() {
				if owner.UID == workload.GetUID() {
					mLog.Info("Find a child resource we are looking for",
						"APIVersion", cr.GetAPIVersion(), "Kind", cr.GetKind(),
						"Name", cr.GetName(), "owner", owner.UID)
					or := cr // have to do a copy as the range variable is a reference and will change
					childResources = append(childResources, &or)
				}
			}
		}
	}
	return childResources, nil
}

// PatchCondition condition for a conditioned object
func PatchCondition(ctx context.Context, r client.StatusClient, workload ConditionedObject,
	condition ...cpv1alpha1.Condition) error {
	workloadPatch := client.MergeFrom(workload.DeepCopyObject())
	workload.SetConditions(condition...)
	return errors.Wrap(
		r.Status().Patch(ctx, workload, workloadPatch, client.FieldOwner(workload.GetUID())),
		ErrUpdateStatus)
}

// A metaObject is a Kubernetes object that has label and annotation
type labelAnnotationObject interface {
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
	GetAnnotations() map[string]string
	SetAnnotations(annotations map[string]string)
}

// PassLabel passes through labels from the parent to the child object
func PassLabel(parentObj oam.Object, childObj labelAnnotationObject) {
	// pass app-config labels
	childObj.SetLabels(MergeMapOverrideWithDst(parentObj.GetLabels(), childObj.GetLabels()))
}

// PassLabelAndAnnotation passes through labels and annotation objectMeta from the parent to the child object
func PassLabelAndAnnotation(parentObj oam.Object, childObj labelAnnotationObject) {
	// pass app-config labels
	childObj.SetLabels(MergeMapOverrideWithDst(parentObj.GetLabels(), childObj.GetLabels()))
	// pass app-config annotation
	childObj.SetAnnotations(MergeMapOverrideWithDst(parentObj.GetAnnotations(), childObj.GetAnnotations()))
}

// GetDefinitionName return the Definition name of any resources
// the format of the definition of a resource is <kind plurals>.<group>
// Now the definition name of a resource could also be defined as `definition.oam.dev/name` in `metadata.annotations`
// typeLabel specified which Definition it is, if specified, will directly get definition from label.
func GetDefinitionName(dm discoverymapper.DiscoveryMapper, u *unstructured.Unstructured, typeLabel string) (string, error) {
	if typeLabel != "" {
		if labels := u.GetLabels(); labels != nil {
			if definitionName, ok := labels[typeLabel]; ok {
				return definitionName, nil
			}
		}
	}
	groupVersion, err := schema.ParseGroupVersion(u.GetAPIVersion())
	if err != nil {
		return "", err
	}
	mapping, err := dm.RESTMapping(schema.GroupKind{Group: groupVersion.Group, Kind: u.GetKind()}, groupVersion.Version)
	if err != nil {
		return "", err
	}
	return mapping.Resource.Resource + "." + groupVersion.Group, nil
}

// GetGVKFromDefinition help get Group Version Kind from DefinitionReference
func GetGVKFromDefinition(dm discoverymapper.DiscoveryMapper, definitionRef v1alpha2.DefinitionReference) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	groupResource := schema.ParseGroupResource(definitionRef.Name)
	gvr := schema.GroupVersionResource{Group: groupResource.Group, Resource: groupResource.Resource, Version: definitionRef.Version}
	kinds, err := dm.KindsFor(gvr)
	if err != nil {
		return gvk, err
	}
	if len(kinds) < 1 {
		return gvk, &meta.NoResourceMatchError{
			PartialResource: gvr,
		}
	}
	return kinds[0], nil
}

// Object2Unstructured convert an object to an unstructured struct
func Object2Unstructured(obj interface{}) (*unstructured.Unstructured, error) {
	objMap, err := Object2Map(obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: objMap,
	}, nil
}

// Object2Map turn the Object to a map
func Object2Map(obj interface{}) (map[string]interface{}, error) {
	var res map[string]interface{}
	bts, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(bts, &res)
	return res, err
}

// GenTraitName generate trait name
func GenTraitName(componentName string, ct *v1alpha2.ComponentTrait, traitType string) string {
	var traitMiddleName = TraitPrefixKey
	if traitType != "" {
		traitMiddleName = strings.ToLower(traitType)
	}
	return fmt.Sprintf("%s-%s-%s", componentName, traitMiddleName, ComputeHash(ct))

}

// ComputeHash returns a hash value calculated from pod template and
// a collisionCount to avoid hash collision. The hash will be safe encoded to
// avoid bad words.
func ComputeHash(trait *v1alpha2.ComponentTrait) string {
	componentTraitHasher := fnv.New32a()
	DeepHashObject(componentTraitHasher, *trait)

	return rand.SafeEncodeString(fmt.Sprint(componentTraitHasher.Sum32()))
}

// DeepHashObject writes specified object to hash using the spew library
// which follows pointers and prints actual values of the nested objects
// ensuring the hash does not change when a pointer changes.
func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	_, _ = printer.Fprintf(hasher, "%#v", objectToWrite)
}

// GetComponent will get Component and RevisionName by AppConfigComponent
func GetComponent(ctx context.Context, client client.Reader, acc v1alpha2.ApplicationConfigurationComponent, namespace string) (*v1alpha2.Component, string, error) {
	c := &v1alpha2.Component{}
	var revisionName string
	if acc.RevisionName != "" {
		revision := &appsv1.ControllerRevision{}
		if err := client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: acc.RevisionName}, revision); err != nil {
			return nil, "", errors.Wrapf(err, errFmtGetComponentRevision, acc.RevisionName)
		}
		c, err := UnpackRevisionData(revision)
		if err != nil {
			return nil, "", errors.Wrapf(err, errFmtControllerRevisionData, acc.RevisionName)
		}
		revisionName = acc.RevisionName
		return c, revisionName, nil
	}
	nn := types.NamespacedName{Namespace: namespace, Name: acc.ComponentName}
	if err := client.Get(ctx, nn, c); err != nil {
		return nil, "", errors.Wrapf(err, errFmtGetComponent, acc.ComponentName)
	}
	if c.Status.LatestRevision != nil {
		revisionName = c.Status.LatestRevision.Name
	}
	return c, revisionName, nil
}

// UnpackRevisionData will unpack revision.Data to Component
func UnpackRevisionData(rev *appsv1.ControllerRevision) (*v1alpha2.Component, error) {
	var err error
	if rev.Data.Object != nil {
		comp, ok := rev.Data.Object.(*v1alpha2.Component)
		if !ok {
			return nil, fmt.Errorf(errFmtInvalidRevisionType, rev.Name, reflect.TypeOf(rev.Data.Object))
		}
		return comp, nil
	}
	var comp v1alpha2.Component
	err = json.Unmarshal(rev.Data.Raw, &comp)
	return &comp, err
}

// AddLabels will merge labels with existing labels. If any conflict keys, use new value to override existing value.
func AddLabels(o *unstructured.Unstructured, labels map[string]string) {
	o.SetLabels(MergeMapOverrideWithDst(o.GetLabels(), labels))
}

// AddAnnotations will merge annotations with existing ones. If any conflict keys, use new value to override existing value.
func AddAnnotations(o *unstructured.Unstructured, annos map[string]string) {
	o.SetAnnotations(MergeMapOverrideWithDst(o.GetAnnotations(), annos))
}

// MergeMapOverrideWithDst merges two could be nil maps. If any conflicts, override src with dst.
func MergeMapOverrideWithDst(src, dst map[string]string) map[string]string {
	if src == nil && dst == nil {
		return nil
	}
	r := make(map[string]string)
	for k, v := range dst {
		r[k] = v
	}
	for k, v := range src {
		if _, exist := r[k]; !exist {
			r[k] = v
		}
	}
	return r
}
