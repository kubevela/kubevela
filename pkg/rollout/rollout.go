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

package rollout

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam"

	kruisev1alpha1 "github.com/openkruise/rollouts/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// ClusterRollout rollout in specified cluster
type ClusterRollout struct {
	*kruisev1alpha1.Rollout
	Cluster string
}

func getAssociatedRollouts(ctx context.Context, cli client.Client, app *v1beta1.Application, withHistoryRTs bool) ([]*ClusterRollout, error) {
	rootRT, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, cli, app)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list resource trackers")
	}
	if !withHistoryRTs {
		historyRTs = []*v1beta1.ResourceTracker{}
	}
	
	seen := make(map[string]struct{})
	var rollouts []*ClusterRollout
	
	for _, rt := range append(historyRTs, rootRT, currentRT) {
		if rt == nil {
			continue
		}
		for _, mr := range rt.Spec.ManagedResources {
			if mr.APIVersion == kruisev1alpha1.SchemeGroupVersion.String() && mr.Kind == "Rollout" {
				key := fmt.Sprintf("%s/%s/%s", mr.Cluster, mr.Namespace, mr.Name)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				
				rollout := &kruisev1alpha1.Rollout{}
				if err = cli.Get(multicluster.ContextWithClusterName(ctx, mr.Cluster), k8stypes.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}, rollout); err != nil {
					if multicluster.IsNotFoundOrClusterNotExists(err) || velaerrors.IsCRDNotExists(err) {
						continue
					}
					return nil, errors.Wrapf(err, "failed to get kruise rollout %s/%s in cluster %s", mr.Namespace, mr.Name, mr.Cluster)
				}
				if value, ok := rollout.Annotations[oam.AnnotationSkipResume]; ok && value == "true" {
					continue
				}
				rollouts = append(rollouts, &ClusterRollout{Rollout: rollout, Cluster: mr.Cluster})
			}
		}
	}
	return rollouts, nil
}

// isCanaryStepPaused returns true if the rollout is paused at a canary step.
// TODO(DependencyUpgrade): openkruise/rollouts v0.3.0 does not include BlueGreenStatus.
// Once KubeVela upgrades the dependency (e.g. to v0.6.0+), BlueGreenStatus should be
// checked here as well since it shares the identical state machine.
func isCanaryStepPaused(r *kruisev1alpha1.Rollout) bool {
	if r.Status.CanaryStatus != nil &&
		r.Status.CanaryStatus.CurrentStepState == kruisev1alpha1.CanaryStepStatePaused {
		return true
	}
	return false
}

// setCanaryStepReady advances the CurrentStepState to CanaryStepStateReady.
// TODO(DependencyUpgrade): Extend this to handle BlueGreenStatus once the Kruise
// rollout API dependency is upgraded to a version that supports it.
func setCanaryStepReady(r *kruisev1alpha1.Rollout) {
	if r.Status.CanaryStatus != nil &&
		r.Status.CanaryStatus.CurrentStepState == kruisev1alpha1.CanaryStepStatePaused {
		r.Status.CanaryStatus.CurrentStepState = kruisev1alpha1.CanaryStepStateReady
	}
}

// SuspendRollout find all rollouts associated with the application (including history RTs) and resume them
func SuspendRollout(ctx context.Context, cli client.Client, app *v1beta1.Application, writer io.Writer) error {
	rollouts, err := getAssociatedRollouts(ctx, cli, app, true)
	if err != nil {
		return err
	}
	for i := range rollouts {
		rollout := rollouts[i]
		if rollout.Status.Phase == kruisev1alpha1.RolloutPhaseProgressing && !rollout.Spec.Strategy.Paused {
			_ctx := multicluster.ContextWithClusterName(ctx, rollout.Cluster)
			rolloutKey := client.ObjectKeyFromObject(rollout.Rollout)
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err = cli.Get(_ctx, rolloutKey, rollout.Rollout); err != nil {
					return err
				}
				if rollout.Status.Phase == kruisev1alpha1.RolloutPhaseProgressing && !rollout.Spec.Strategy.Paused {
					rollout.Spec.Strategy.Paused = true
					if err = cli.Update(_ctx, rollout.Rollout); err != nil {
						return err
					}
					if writer != nil {
						_, _ = fmt.Fprintf(writer, "Rollout %s/%s in cluster %s suspended.\n", rollout.Namespace, rollout.Name, rollout.Cluster)
					}
					return nil
				}
				return nil
			}); err != nil {
				return errors.Wrapf(err, "failed to suspend rollout %s/%s in cluster %s", rollout.Namespace, rollout.Name, rollout.Cluster)
			}
		}
	}
	return nil
}

// ResumeRollout find all rollouts associated with the application (in the current RT) and resume them
func ResumeRollout(ctx context.Context, cli client.Client, app *v1beta1.Application, writer io.Writer) (bool, error) {
	rollouts, err := getAssociatedRollouts(ctx, cli, app, false)
	if err != nil {
		return false, err
	}
	modified := false
	for i := range rollouts {
		rollout := rollouts[i]
		if rollout.Spec.Strategy.Paused || isCanaryStepPaused(rollout.Rollout) {
			_ctx := multicluster.ContextWithClusterName(ctx, rollout.Cluster)
			rolloutKey := client.ObjectKeyFromObject(rollout.Rollout)
			resumed := false
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err = cli.Get(_ctx, rolloutKey, rollout.Rollout); err != nil {
					return err
				}
				if rollout.Spec.Strategy.Paused {
					rollout.Spec.Strategy.Paused = false
					if err = cli.Update(_ctx, rollout.Rollout); err != nil {
						return err
					}
					resumed = true
					return nil
				}
				return nil
			}); err != nil {
				return false, errors.Wrapf(err, "failed to resume rollout %s/%s in cluster %s", rollout.Namespace, rollout.Name, rollout.Cluster)
			}
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err = cli.Get(_ctx, rolloutKey, rollout.Rollout); err != nil {
					return err
				}
				if isCanaryStepPaused(rollout.Rollout) {
					setCanaryStepReady(rollout.Rollout)
					if err = cli.Status().Update(_ctx, rollout.Rollout); err != nil {
						return err
					}
					resumed = true
					return nil
				}
				return nil
			}); err != nil {
				return false, errors.Wrapf(err, "failed to resume rollout %s/%s in cluster %s", rollout.Namespace, rollout.Name, rollout.Cluster)
			}
			if resumed {
				modified = true
				if writer != nil {
					_, _ = fmt.Fprintf(writer, "Rollout %s/%s in cluster %s resumed.\n", rollout.Namespace, rollout.Name, rollout.Cluster)
				}
			}
		}
	}
	return modified, nil
}

// RollbackRollout find all rollouts associated with the application (in the current RT) and disable the pause field.
func RollbackRollout(ctx context.Context, cli client.Client, app *v1beta1.Application, writer io.Writer) (bool, error) {
	rollouts, err := getAssociatedRollouts(ctx, cli, app, false)
	if err != nil {
		return false, err
	}
	modified := false
	for i := range rollouts {
		rollout := rollouts[i]
		if rollout.Spec.Strategy.Paused || isCanaryStepPaused(rollout.Rollout) {
			_ctx := multicluster.ContextWithClusterName(ctx, rollout.Cluster)
			rolloutKey := client.ObjectKeyFromObject(rollout.Rollout)
			resumed := false
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err = cli.Get(_ctx, rolloutKey, rollout.Rollout); err != nil {
					return err
				}
				if rollout.Spec.Strategy.Paused {
					rollout.Spec.Strategy.Paused = false
					if err = cli.Update(_ctx, rollout.Rollout); err != nil {
						return err
					}
					resumed = true
					return nil
				}
				return nil
			}); err != nil {
				return false, errors.Wrapf(err, "failed to rollback rollout %s/%s in cluster %s", rollout.Namespace, rollout.Name, rollout.Cluster)
			}
			if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err = cli.Get(_ctx, rolloutKey, rollout.Rollout); err != nil {
					return err
				}
				if isCanaryStepPaused(rollout.Rollout) {
					setCanaryStepReady(rollout.Rollout)
					if err = cli.Status().Update(_ctx, rollout.Rollout); err != nil {
						return err
					}
					resumed = true
					return nil
				}
				return nil
			}); err != nil {
				return false, errors.Wrapf(err, "failed to rollback rollout %s/%s in cluster %s", rollout.Namespace, rollout.Name, rollout.Cluster)
			}
			if resumed {
				modified = true
				if writer != nil {
					_, _ = fmt.Fprintf(writer, "Rollout %s/%s in cluster %s rollback.\n", rollout.Namespace, rollout.Name, rollout.Cluster)
				}
			}
		}
	}
	return modified, nil
}
