package addon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// objectCollector accumulates the objects the installer applied, for recording
// into the addon's drift ResourceTracker.
type objectCollector struct {
	mu   sync.Mutex
	objs []client.Object
}

func newObjectCollector() *objectCollector { return &objectCollector{} }

func (c *objectCollector) sink(o client.Object) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// DeepCopyObject returns runtime.Object; every object the installer emits is a
	// client.Object, but guard the assertion so a future non-client.Object emitter
	// drops the object rather than panicking the controller.
	copied, ok := o.DeepCopyObject().(client.Object)
	if !ok {
		return
	}
	c.objs = append(c.objs, copied)
}

func (c *objectCollector) objects() []client.Object {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]client.Object, len(c.objs))
	copy(out, c.objs)
	return out
}

// errSourceUnresolved marks failures where the addon source could not be
// reached or resolved (registry list/connect failure, or a fetch error). These
// drive SourceResolved=False with backoff rather than an immediate failed phase.
var errSourceUnresolved = errors.New("addon source could not be resolved")

// isRegistryUnreachable reports whether err is a source-resolution failure.
func isRegistryUnreachable(err error) bool {
	return errors.Is(err, errSourceUnresolved)
}

// buildArgs converts spec.parameters (free-form) into installer args.
func buildArgs(ad *v1beta1.Addon) (map[string]any, error) {
	if ad.Spec.Parameters == nil || len(ad.Spec.Parameters.Raw) == 0 {
		return nil, nil
	}
	var args map[string]any
	if err := json.Unmarshal(ad.Spec.Parameters.Raw, &args); err != nil {
		return nil, fmt.Errorf("invalid spec.parameters: %w", err)
	}
	return args, nil
}

// resolveRegistry finds the named registry and its index in the full list (the
// index drives dependency-registry filtering, matching the CLI install path).
func (r *Reconciler) resolveRegistry(ctx context.Context, name string) (pkgaddon.Registry, []pkgaddon.Registry, int, error) {
	ds := pkgaddon.NewRegistryDataStore(r.Client)
	registries, err := ds.ListRegistries(ctx)
	if err != nil {
		return pkgaddon.Registry{}, nil, -1, fmt.Errorf("%w: list registries: %w", errSourceUnresolved, err)
	}
	for i, reg := range registries {
		if reg.Name == name {
			return reg, registries, i, nil
		}
	}
	return pkgaddon.Registry{}, nil, -1, fmt.Errorf("%w: registry %q not registered", errSourceUnresolved, name)
}

// install delegates to the shared installer (the same EnableAddon vela addon
// enable uses). Fetch errors are classified as source-unresolved for backoff.
func (r *Reconciler) install(ctx context.Context, ad *v1beta1.Addon) error {
	args, err := buildArgs(ad)
	if err != nil {
		return err
	}
	registry, registries, i, err := r.resolveRegistry(ctx, ad.Spec.Registry)
	if err != nil {
		return err
	}
	collector := newObjectCollector()
	opts := append(installOptions(ad), pkgaddon.WithAppliedObjectSink(collector.sink))
	_, err = pkgaddon.EnableAddon(ctx, ad.Name, ad.Spec.Version,
		r.Client, r.DiscoveryClient, apply.NewAPIApplicator(r.Client), r.Config,
		registry, args, nil, // cache: fetch every reconcile
		pkgaddon.FilterDependencyRegistries(i, registries), opts...)
	if err != nil {
		if errors.Is(err, pkgaddon.ErrFetch) {
			return fmt.Errorf("%w: %w", errSourceUnresolved, err)
		}
		return err
	}
	// Record everything the installer applied (the Application plus each
	// auxiliary) in the addon-owned drift ResourceTracker, so steady-state
	// reconciles can heal drift from the stored manifests without a registry
	// fetch.
	return r.writeTracker(ctx, ad, collector.objects())
}

// installOptions maps Addon spec flags onto the installer's InstallOptions.
func installOptions(ad *v1beta1.Addon) []pkgaddon.InstallOption {
	var opts []pkgaddon.InstallOption
	if ad.Spec.SkipVersionCheck {
		// Skip the minKubeVelaVersion / running-instance check. Needed when the
		// controller runs out-of-cluster (e.g. from an IDE) against a cluster
		// that has the CRDs and definitions but no vela-core Deployment to read
		// a version from.
		opts = append(opts, pkgaddon.SkipValidateVersion)
	}
	if ad.Spec.OverrideDefinitions {
		opts = append(opts, pkgaddon.OverrideDefinitions)
	}
	return opts
}

// readBackStatus reflects the installed Application into status fields.
func (r *Reconciler) readBackStatus(ctx context.Context, ad *v1beta1.Addon) error {
	st, err := pkgaddon.GetAddonStatus(ctx, r.Client, ad.Name)
	if err != nil {
		return err
	}
	ad.Status.ApplicationName = addonutil.Addon2AppName(ad.Name)
	ad.Status.InstalledVersion = st.InstalledVersion
	ad.Status.InstalledRegistry = st.InstalledRegistry
	return nil
}

// sourceResolvedStaleFor reports whether SourceResolved has been False for at
// least d, used to escalate persistent fetch failure to phase=failed.
func sourceResolvedStaleFor(ad *v1beta1.Addon, d time.Duration) bool {
	for i := range ad.Status.Conditions {
		c := ad.Status.Conditions[i]
		if c.Type == v1beta1.AddonConditionSourceResolved {
			if c.Status != metav1.ConditionFalse {
				return false
			}
			return time.Since(c.LastTransitionTime.Time) >= d
		}
	}
	return false
}
