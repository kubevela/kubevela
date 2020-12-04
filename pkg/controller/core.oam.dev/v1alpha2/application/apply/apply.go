package apply

import (
	"context"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Object interface
type Object interface {
	metav1.Object
	runtime.Object
}

// Option is some configuration that modifies options for a apply.
type Option func(o *options)

// DanglingPolicy specify a policy for handling dangling cr
func DanglingPolicy(policy string) Option {
	return func(o *options) {
		o.danglingPolicy = policy
	}
}

// SetOwnerReferences set OwnerReferences
func SetOwnerReferences(refs []metav1.OwnerReference) Option {
	return func(o *options) {
		o.ownerRefs = refs
	}
}

// List Set the function to get the object.
func List(lister Lister) Option {
	return func(o *options) {
		o.lister = lister
	}
}

// Lister Get the current objects
type Lister func(cli client.Client) ([]Object, error)

type options struct {
	danglingPolicy string
	lister         Lister
	ownerRefs      []metav1.OwnerReference
}

// Syncer synchronize the current state to the desired state
type Syncer struct {
	client client.Client
}

// New Syncer
func New(cli client.Client) *Syncer {
	return &Syncer{cli}
}

// Apply perform synchronization operations
func (syncer *Syncer) Apply(objs []Object, applyOptions ...Option) error {
	opts := &options{}
	for _, applyOpt := range applyOptions {
		applyOpt(opts)
	}
	if opts.lister != nil {
		oldObjs, err := opts.lister(syncer.client)
		if err != nil {
			return err
		}
		for index := range oldObjs {
			oldObj := oldObjs[index]
			dangling := true
			for _, obj := range objs {
				if isOne(oldObj, obj) {
					dangling = false
					break
				}
			}
			if dangling {
				if opts.danglingPolicy == "delete" {
					if err := syncer.client.Delete(context.Background(), oldObj); err != nil {
						return err
					}
				}
			}
		}

	}
	ctx := context.Background()
	for _, obj := range objs {
		if opts.ownerRefs != nil {
			obj.SetOwnerReferences(opts.ownerRefs)
		}
		if err := syncer.client.Create(ctx, obj); err != nil {
			if !kerrors.IsAlreadyExists(err) {
				return err
			}
			current := obj.DeepCopyObject()
			if err := syncer.client.Get(ctx, client.ObjectKey{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, current); err != nil {
				return err
			}
			obj.SetResourceVersion(current.(metav1.Object).GetResourceVersion())
			if err := syncer.client.Update(ctx, obj); err != nil {
				return err
			}
		}
	}
	return nil
}

func isOne(src, dest Object) bool {
	if src.GetObjectKind().GroupVersionKind().String() !=
		dest.GetObjectKind().GroupVersionKind().String() {
		return false
	}
	if src.GetName() != dest.GetName() {
		return false
	}
	return true
}
