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

package resourcetracker

import (
	"context"
	"fmt"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/kubeutil"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// Tracked is whoever a set of ResourceTrackers, and the resources they record, belongs to.
// Each kind of owner implements it, embedding Base and overriding only where it differs;
// the constructors live with the kind, not in this package.
type Tracked interface {
	Kind() string
	Name() string
	Namespace() string
	// Key identifies the owner in ownership checks and sharer lists.
	Key() string
	// Deleting reports that the owner itself is being deleted, so all its trackers are inactive.
	Deleting() bool
	// ControlledBy returns the key of whoever controls obj, or "" if nobody does.
	ControlledBy(obj client.Object) string
	// Stamp marks a manifest as owned by this owner before it is applied.
	Stamp(obj *unstructured.Unstructured)
	// Release removes the marks Stamp (or the owner's kind) put on a resource that is being
	// left behind rather than deleted.
	Release(obj *unstructured.Unstructured)
	// Orphaning reports that the owner's resources are to be left behind, not deleted,
	// when it lets them go.
	Orphaning() bool
	// TrackerLabels are the labels every tracker of the owner carries.
	TrackerLabels() map[string]string
	// NewTracker returns a tracker of the given type, named and typed for this owner and not
	// yet created. CreateTracker adds TrackerLabels to whatever it returns, so an
	// implementation adds only the labels and annotations peculiar to it.
	NewTracker(rtType v1beta1.ResourceTrackerType) (*v1beta1.ResourceTracker, error)
	// LoadTrackers finds the owner's existing trackers.
	LoadTrackers(ctx context.Context, cli client.Client) (Trackers, error)
}

// Trackers are an owner's trackers: the root (life-long resources), the current version,
// older versions sorted oldest first, and any component-revision tracker.
type Trackers struct {
	Root              *v1beta1.ResourceTracker
	Current           *v1beta1.ResourceTracker
	History           []*v1beta1.ResourceTracker
	ComponentRevision *v1beta1.ResourceTracker
}

// CreateTracker creates a new tracker of the given type for t.
func CreateTracker(ctx context.Context, cli client.Client, t Tracked, rtType v1beta1.ResourceTrackerType) (*v1beta1.ResourceTracker, error) {
	rt, err := t.NewTracker(rtType)
	if err != nil {
		return nil, err
	}
	// Labelled here rather than in NewTracker: embedding gives no virtual dispatch, so a kind
	// reusing Base.NewTracker would lose the labels its own TrackerLabels adds.
	meta.AddLabels(rt, t.TrackerLabels())
	meta.AddFinalizer(rt, Finalizer)
	setCompression(rt)
	if err := cli.Create(ctx, rt); err != nil {
		return nil, err
	}
	return rt, nil
}

// Base is the behaviour every kind shares: owner.oam.dev/* labels on trackers and on owned
// resources, trackers named <namespace>-<name>[-v<generation>], and the current tracker
// chosen by generation.
type Base struct {
	kind, name, namespace string
	uid                   types.UID
	generation            int64
	deleting              bool
}

// Owner identifies the owner a Base speaks for. A struct rather than parameters: kind,
// namespace and name are all strings, and swapping two of them compiles.
type Owner struct {
	Kind, Namespace, Name string
	UID                   types.UID
	Generation            int64
	// Deleting reports that the owner itself is being deleted.
	Deleting bool
}

// NewBase returns the shared behaviour for an owner.
func NewBase(o Owner) *Base {
	return &Base{kind: o.Kind, name: o.Name, namespace: o.Namespace, uid: o.UID, generation: o.Generation, deleting: o.Deleting}
}

// Kind of the owner.
func (b *Base) Kind() string { return b.kind }

// Name of the owner.
func (b *Base) Name() string { return b.name }

// Namespace of the owner.
func (b *Base) Namespace() string { return b.namespace }

// Deleting reports that the owner is being deleted.
func (b *Base) Deleting() bool { return b.deleting }

// Key is <kind>/<namespace>/<name>.
func (b *Base) Key() string { return fmt.Sprintf("%s/%s/%s", b.kind, b.namespace, b.name) }

// Labels select the owner's trackers and mark the resources it owns.
func (b *Base) Labels() map[string]string {
	return map[string]string{
		oam.LabelOwnerKind:      b.kind,
		oam.LabelOwnerName:      b.name,
		oam.LabelOwnerNamespace: b.namespace,
	}
}

// ControlledBy reads the owner.oam.dev/* labels.
func (b *Base) ControlledBy(obj client.Object) string {
	return OwnerKeyFromLabels(obj.GetLabels())
}

// HasAnyOwnerLabel reports whether labels carry any owner.oam.dev/* mark, so a kind whose
// own marks are absent can tell "nobody has claimed this" from "somebody did, incompletely".
func HasAnyOwnerLabel(labels map[string]string) bool {
	for _, k := range ownerLabelKeys {
		if labels[k] != "" {
			return true
		}
	}
	return false
}

// OwnerKeyFromLabels returns the key the owner.oam.dev/* labels name, "" if they name none.
// Kinds read their labels once and pass them here, since reading them copies the map.
func OwnerKeyFromLabels(labels map[string]string) string {
	kind, name, ns := labels[oam.LabelOwnerKind], labels[oam.LabelOwnerName], labels[oam.LabelOwnerNamespace]
	if kind == "" || name == "" || ns == "" {
		return ""
	}
	return kind + "/" + ns + "/" + name
}

// Stamp adds the owner.oam.dev/* labels.
func (b *Base) Stamp(obj *unstructured.Unstructured) {
	kubeutil.AddLabels(obj, b.Labels())
}

// Release removes the owner.oam.dev/* labels.
func (b *Base) Release(obj *unstructured.Unstructured) {
	kubeutil.RemoveLabels(obj, ownerLabelKeys)
}

// Orphaning is false: by default an owner's resources are deleted with it.
func (b *Base) Orphaning() bool { return false }

var ownerLabelKeys = []string{oam.LabelOwnerKind, oam.LabelOwnerName, oam.LabelOwnerNamespace}

// LabelsForKey returns the labels that mark a resource as controlled by the owner with this
// key: owner.oam.dev/* and the kind's own (Config.KindLabels). The key may be in either form
// (see apply.CanonicalOwnerKey); nil if it names no kind.
func LabelsForKey(key string) map[string]string {
	parts := strings.Split(apply.CanonicalOwnerKey(key), "/")
	if len(parts) != 3 {
		return nil
	}
	kind, namespace, name := parts[0], parts[1], parts[2]
	labels := map[string]string{oam.LabelOwnerKind: kind, oam.LabelOwnerNamespace: namespace, oam.LabelOwnerName: name}
	if f := CurrentConfig().KindLabels; f != nil {
		for k, v := range f(kind, namespace, name) {
			labels[k] = v
		}
	}
	return labels
}

// OwnerLabelKeys are the labels Base puts on the resources it owns.
func OwnerLabelKeys() []string { return append([]string(nil), ownerLabelKeys...) }

// TrackerLabels are the owner.oam.dev/* labels, including the owner's uid.
func (b *Base) TrackerLabels() map[string]string {
	labels := b.Labels()
	labels[oam.LabelOwnerUID] = string(b.uid)
	return labels
}

// NewTracker names a root or versioned tracker. Other types belong to the kind.
func (b *Base) NewTracker(rtType v1beta1.ResourceTrackerType) (*v1beta1.ResourceTracker, error) {
	rt := &v1beta1.ResourceTracker{}
	switch rtType {
	case v1beta1.ResourceTrackerTypeRoot:
		rt.SetName(fmt.Sprintf("%s-%s", b.namespace, b.name))
	case v1beta1.ResourceTrackerTypeVersioned:
		rt.SetName(fmt.Sprintf("%s-%s-v%d", b.namespace, b.name, b.generation))
		rt.Spec.ApplicationGeneration = b.generation
	default:
		return nil, errors.Errorf("%s %s has no %s resource tracker", b.kind, b.Key(), rtType)
	}
	rt.Spec.Type = rtType
	return rt, nil
}

// LoadTrackers lists the owner's trackers by its owner.oam.dev/* labels.
func (b *Base) LoadTrackers(ctx context.Context, cli client.Client) (Trackers, error) {
	OnList(b.kind)
	rts := v1beta1.ResourceTrackerList{}
	if err := cli.List(ctx, &rts, client.MatchingLabels(b.Labels())); err != nil {
		return Trackers{}, errors.WithMessage(err, "failed to list ResourceTrackers")
	}
	var trackers Trackers
	for _, _rt := range rts.Items {
		rt := _rt.DeepCopy()
		if uid := rt.GetLabels()[oam.LabelOwnerUID]; uid != "" && uid != string(b.uid) {
			return Trackers{}, errors.Errorf("resourcetracker %s exists but is controlled by another %s %s (uid: %s)", rt.Name, b.kind, b.Key(), uid)
		}
		switch rt.Spec.Type {
		case v1beta1.ResourceTrackerTypeRoot:
			trackers.Root = rt
		case v1beta1.ResourceTrackerTypeVersioned:
			if rt.Spec.ApplicationGeneration == b.generation {
				trackers.Current = rt
			} else {
				trackers.History = append(trackers.History, rt)
			}
		default:
		}
	}
	trackers.History = SortResourceTrackersByVersion(trackers.History, false)
	return trackers, nil
}
