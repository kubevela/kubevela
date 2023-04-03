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
	var rollouts []*ClusterRollout
	for _, rt := range append(historyRTs, rootRT, currentRT) {
		if rt == nil {
			continue
		}
		for _, mr := range rt.Spec.ManagedResources {
			if mr.APIVersion == kruisev1alpha1.SchemeGroupVersion.String() && mr.Kind == "Rollout" {
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
		if rollout.Spec.Strategy.Paused || (rollout.Status.CanaryStatus != nil && rollout.Status.CanaryStatus.CurrentStepState == kruisev1alpha1.CanaryStepStatePaused) {
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
				if rollout.Status.CanaryStatus != nil && rollout.Status.CanaryStatus.CurrentStepState == kruisev1alpha1.CanaryStepStatePaused {
					rollout.Status.CanaryStatus.CurrentStepState = kruisev1alpha1.CanaryStepStateReady
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
		if rollout.Spec.Strategy.Paused || (rollout.Status.CanaryStatus != nil && rollout.Status.CanaryStatus.CurrentStepState == kruisev1alpha1.CanaryStepStatePaused) {
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
