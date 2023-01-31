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

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/kubevela/pkg/multicluster"
	"github.com/kubevela/pkg/util/slices"
	"github.com/spf13/cobra"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	com "github.com/oam-dev/kubevela/references/common"
)

// DeleteOptions options for vela delete command
type DeleteOptions struct {
	AppNames  []string
	Namespace string

	All         bool
	Wait        bool
	Orphan      bool
	Force       bool
	Interactive bool

	AssumeYes bool
}

// Complete .
func (opt *DeleteOptions) Complete(f velacmd.Factory, cmd *cobra.Command, args []string) error {
	opt.AppNames = args
	opt.Namespace = velacmd.GetNamespace(f, cmd)
	opt.AssumeYes = assumeYes
	if len(opt.AppNames) > 0 && opt.All {
		return fmt.Errorf("application name and --all cannot be both set")
	}
	if opt.All {
		apps := &v1beta1.ApplicationList{}
		if err := f.Client().List(cmd.Context(), apps, client.InNamespace(opt.Namespace)); err != nil {
			return fmt.Errorf("failed to load application in namespace %s: %w", opt.Namespace, err)
		}
		opt.AppNames = slices.Map(apps.Items, func(app v1beta1.Application) string { return app.Name })
	}
	return nil
}

// Validate validate if vela delete args are valid
func (opt *DeleteOptions) Validate() error {
	switch {
	case len(opt.AppNames) == 0 && !opt.All:
		return fmt.Errorf("no application provided for deletion")
	case len(opt.AppNames) == 0 && opt.All:
		return fmt.Errorf("no application found in namespace %s for deletion", opt.Namespace)
	case opt.Interactive && (opt.Force || opt.Orphan):
		return fmt.Errorf("--interactive cannot be used together with --force and --orphan")
	}
	return nil
}

func (opt *DeleteOptions) getDeletingStatus(ctx context.Context, f velacmd.Factory, appKey apitypes.NamespacedName) (done bool, msg string, err error) {
	app := &v1beta1.Application{}
	err = f.Client().Get(ctx, appKey, app)
	switch {
	case kerrors.IsNotFound(err):
		return true, "", nil
	case err != nil:
		return false, "", err
	case app.DeletionTimestamp == nil:
		return false, "application deletion is not handled by apiserver yet", nil
	case app.Status.Phase != common.ApplicationDeleting:
		return false, "application deletion is not handled by controller yet", nil
	default:
		if cond := slices.Find(app.Status.Conditions, func(cond condition.Condition) bool { return cond.Reason == condition.ReasonDeleting }); cond != nil {
			return false, cond.Message, nil
		}
		return false, "", nil
	}
}

// DeleteApp delete one application
func (opt *DeleteOptions) DeleteApp(f velacmd.Factory, cmd *cobra.Command, app *v1beta1.Application) error {
	ctx := cmd.Context()

	// delete the application interactively
	if opt.Interactive {
		if err := opt.interactiveDelete(ctx, f, cmd, app); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Exit interactive deletion mode. You can switch to normal mode and continue with automatic deletion.\n")
		return nil
	}

	if !opt.AssumeYes {
		if !NewUserInput().AskBool(fmt.Sprintf("Are you sure to delete the application %s/%s", app.Namespace, app.Name), &UserInputOptions{opt.AssumeYes}) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "skip deleting appplication %s/%s\n", app.Namespace, app.Name)
			return nil
		}
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Start deleting appplication %s/%s\n", app.Namespace, app.Name)

	// orphan app
	if opt.Orphan {
		if err := opt.orphan(ctx, f, app); err != nil {
			return err
		}
	}

	// delete app
	if app.DeletionTimestamp == nil {
		if err := opt.delete(ctx, f, app); err != nil {
			return err
		}
	}

	// force delete the application
	if opt.Force {
		if err := com.PrepareToForceDeleteTerraformComponents(ctx, f.Client(), app.Namespace, app.Name); err != nil {
			return err
		}
		if err := opt.forceDelete(ctx, f, app); err != nil {
			return err
		}
	}

	// wait for deletion finished
	if opt.Wait {
		if err := opt.wait(ctx, f, app); err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Delete appplication %s/%s succeeded\n", app.Namespace, app.Name)
	return nil
}

func (opt *DeleteOptions) orphan(ctx context.Context, f velacmd.Factory, app *v1beta1.Application) error {
	if !slices.Contains(app.GetFinalizers(), oam.FinalizerOrphanResource) {
		meta.AddFinalizer(app, oam.FinalizerOrphanResource)
		if err := f.Client().Update(ctx, app); err != nil {
			return fmt.Errorf("failed to set orphan resource finalizer to application %s/%s: %w", app.Namespace, app.Name, err)
		}
	}
	return nil
}

func (opt *DeleteOptions) forceDelete(ctx context.Context, f velacmd.Factory, app *v1beta1.Application) error {
	return wait.PollImmediate(3*time.Second, 1*time.Minute, func() (done bool, err error) {
		err = f.Client().Get(ctx, client.ObjectKeyFromObject(app), app)
		if kerrors.IsNotFound(err) {
			return true, nil
		}
		rk, err := resourcekeeper.NewResourceKeeper(ctx, f.Client(), app)
		if err != nil {
			return false, fmt.Errorf("failed to create resource keeper to run garbage collection: %w", err)
		}
		if done, _, err = rk.GarbageCollect(ctx); err != nil && !kerrors.IsConflict(err) {
			return false, fmt.Errorf("failed to run garbage collect: %w", err)
		}
		if done {
			meta.RemoveFinalizer(app, oam.FinalizerResourceTracker)
			meta.RemoveFinalizer(app, oam.FinalizerOrphanResource)
			if err = f.Client().Update(ctx, app); err != nil && !kerrors.IsConflict(err) && !kerrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to update app finalizer: %w", err)
			}
		}
		return false, nil
	})
}

func (opt *DeleteOptions) deleteResource(ctx context.Context, f velacmd.Factory, mr v1beta1.ManagedResource, app *v1beta1.Application) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(mr.GroupVersionKind())
	if err := f.Client().Get(multicluster.WithCluster(ctx, mr.Cluster), mr.NamespacedName(), obj); err != nil {
		return client.IgnoreNotFound(err)
	}
	if !resourcekeeper.IsResourceManagedByApplication(obj, app) {
		return nil
	}
	return resourcekeeper.DeleteManagedResourceInApplication(ctx, f.Client(), mr, obj, app)
}

func _getManagedResourceSource(mr v1beta1.ManagedResource) string {
	src := "in cluster local"
	if mr.Cluster != "" {
		src = fmt.Sprintf("in cluster %s", mr.Cluster)
	}
	if mr.Namespace != "" {
		src += fmt.Sprintf(", namespace %s", mr.Namespace)
	}
	groups := strings.Split(mr.APIVersion, "/")
	group := "." + groups[0]
	if len(groups) == 0 {
		group = ""
	}
	return fmt.Sprintf("%s%s %s %s", strings.ToLower(mr.Kind), group, mr.Name, src)
}

func (opt *DeleteOptions) interactiveDelete(ctx context.Context, f velacmd.Factory, cmd *cobra.Command, app *v1beta1.Application) error {
	for {
		rootRT, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, f.Client(), app)
		if err != nil {
			return fmt.Errorf("failed to get ResourceTrackers for application %s/%s: %w", app.Namespace, app.Name, err)
		}
		rts := slices.Filter(append(historyRTs, currentRT, rootRT), func(rt *v1beta1.ResourceTracker) bool { return rt != nil })
		rs := map[string]v1beta1.ManagedResource{}
		for _, rt := range rts {
			for _, mr := range rt.Spec.ManagedResources {
				rs[_getManagedResourceSource(mr)] = mr
			}
		}
		var opts []string
		for k := range rs {
			opts = append(opts, k)
		}
		if len(opts) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No resources found for application %s/%s\n", app.Namespace, app.Name)
			return nil
		}
		prompt := &survey.Select{
			Message: "Please choose which resource to delete",
			Options: append(opts, "exit"),
		}
		var choice string
		if err = survey.AskOne(prompt, &choice); err != nil {
			return fmt.Errorf("exit on error: %w", err)
		}
		if choice == "exit" {
			break
		}
		mr := rs[choice]
		if err = opt.deleteResource(ctx, f, mr, app); err != nil {
			if !NewUserInput().AskBool(fmt.Sprintf("Error encountered while recycling %s: %s.\nDo you want to skip this error?", choice, err.Error()), &UserInputOptions{AssumeYes: opt.AssumeYes}) {
				return fmt.Errorf("deletion aborted")
			}
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Successfully recycled resource %s\n", choice)
		}
		for _, rt := range rts {
			if slices.Index(rt.Spec.ManagedResources, func(r v1beta1.ManagedResource) bool { return r.ResourceKey() == mr.ResourceKey() }) >= 0 {
				rt.Spec.ManagedResources = slices.Filter(rt.Spec.ManagedResources, func(r v1beta1.ManagedResource) bool { return r.ResourceKey() != mr.ResourceKey() })
				if err = f.Client().Update(ctx, rt); err != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Error encountered when updating ResourceTracker %s: %s\n", rt.Name, err.Error())
				}
			}
		}
	}
	return nil
}

func (opt *DeleteOptions) delete(ctx context.Context, f velacmd.Factory, app *v1beta1.Application) error {
	if err := f.Client().Delete(ctx, app); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete application %s/%s: %w", app.Namespace, app.Name, err)
	}
	return nil
}

func (opt *DeleteOptions) wait(ctx context.Context, f velacmd.Factory, app *v1beta1.Application) error {
	spinner := newTrackingSpinnerWithDelay(fmt.Sprintf("deleting application %s/%s", app.Namespace, app.Name), time.Second)
	spinner.Start()
	defer spinner.Stop()
	return wait.PollImmediate(2*time.Second, 5*time.Minute, func() (done bool, err error) {
		var msg string
		done, msg, err = opt.getDeletingStatus(ctx, f, client.ObjectKeyFromObject(app))
		applySpinnerNewSuffix(spinner, msg)
		return done, err
	})
}

// Run vela delete
func (opt *DeleteOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
	for _, appName := range opt.AppNames {
		app := &v1beta1.Application{}
		if err := f.Client().Get(cmd.Context(), apitypes.NamespacedName{Namespace: opt.Namespace, Name: appName}, app); err != nil {
			if kerrors.IsNotFound(err) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "application %s/%s already deleted\n", opt.Namespace, appName)
				return nil
			}
			return fmt.Errorf("failed to get application %s/%s: %w", opt.Namespace, appName, err)
		}
		if err := opt.DeleteApp(f, cmd, app); err != nil {
			return err
		}
	}
	return nil
}

var (
	deleteLong = templates.LongDesc(i18n.T(`
		Delete applications

		Delete KubeVela applications. KubeVela application deletion is associated
		with the recycle of underlying resources. By default, the resources created
		by the KubeVela application will be deleted once it is not in use or the
		application is deleted. There is garbage-collect policy in KubeVela application
		that you can use to configure customized recycle rules.

		This command supports delete application in various modes.
		Natively, you can use it like "kubectl delete app [app-name]". 
		In the cases you only want to delete the application but leave the 
		resources there, you can use the --orphan parameter.
		In the cases the server-side controller is uninstalled, or you want to
		manually skip some errors in the deletion process (like lack privileges or
		handle cluster disconnection), you can use the --force parameter. 
	`))

	deleteExample = templates.Examples(i18n.T(`
		# Delete an application
		vela delete my-app
	
		# Delete multiple applications in a namespace
		vela delete app-1 app-2 -n example

		# Delete all applications in one namespace
		vela delete -n example --all

		# Delete application without waiting to be deleted
		vela delete my-app --wait=false

		# Delete application without confirmation
		vela delete my-app -y

		# Force delete application at client-side
		vela delete my-app -f

		# Delete application by orphaning resources and skip recycling them
		vela delete my-app --orphan

		# Delete application interactively
		vela delete my-app -i
	`))
)

// NewDeleteCommand Delete App
func NewDeleteCommand(f velacmd.Factory, order string) *cobra.Command {
	o := &DeleteOptions{
		Wait: true,
	}
	cmd := &cobra.Command{
		Use:                   "delete",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Delete an application"),
		Long:                  deleteLong,
		Example:               deleteExample,
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd, args))
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}

	cmd.PersistentFlags().BoolVarP(&o.Wait, "wait", "w", o.Wait, "wait util the application is deleted completely")
	cmd.PersistentFlags().BoolVarP(&o.All, "all", "", o.All, "delete all the application under the given namespace")
	cmd.PersistentFlags().BoolVarP(&o.Orphan, "orphan", "o", o.Orphan, "delete the application and orphan managed resources")
	cmd.PersistentFlags().BoolVarP(&o.Force, "force", "f", o.Force, "force delete the application")
	cmd.PersistentFlags().BoolVarP(&o.Interactive, "interactive", "i", o.Interactive, "delete the application interactively")

	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag().
		WithResponsiveWriter().
		Build()
}
