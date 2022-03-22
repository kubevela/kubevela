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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/pkg/errors"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func diffString(original, current string) string {
	dmp := diffmatchpatch.New()
	odmp, cdmp, dmpStrings := dmp.DiffLinesToChars(original, current)
	diffs := dmp.DiffMain(odmp, cdmp, false)
	diffs = dmp.DiffCharsToLines(diffs, dmpStrings)
	diffs = dmp.DiffCleanupSemantic(diffs)
	s := dmp.DiffPrettyText(diffs)
	return s
}

// NewDiffCommand command for comparing current application's spec and the spec used in the last workflow run
func NewDiffCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff APP_NAME",
		Short: "show the differences for application since last workflow run",
		Long:  "show the differences between the current application spec and the application revision in last workflow run",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Args: cobra.RangeArgs(1, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			cli, err := c.GetClient()
			if err != nil {
				return err
			}

			name := args[0]
			app := &v1beta1.Application{}
			ctx := context.Background()
			if err = cli.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: name}, app); err != nil {
				return errors.Wrapf(err, "failed to get application %s/%s", namespace, name)
			}
			publishVersion := oam.GetPublishVersion(app)
			if publishVersion == "" {
				cmd.Printf("Application is not running with PublishVersion, the workflow always runs with the latest spec.\n")
				return nil
			}
			revs, err := application.GetAppRevisions(ctx, cli, app.Name, app.Namespace)
			if err != nil {
				return errors.Wrapf(err, "failed to get revisions for application %s/%s", app.Namespace, app.Name)
			}
			appParser, err := getAppParser(c, cli)
			if err != nil {
				return errors.Wrapf(err, "failed to get app parser")
			}

			var af1, af2 *appfile.Appfile
			var spec1, spec2 v1beta1.ApplicationSpec

			load := func(revName string, searchPublishVersion bool, afPtr **appfile.Appfile, specPtr *v1beta1.ApplicationSpec) error {
				rev, err := findRevInAppRevs(revName, revs, searchPublishVersion)
				if err != nil {
					return err
				}
				af, err := appParser.GenerateAppFileFromRevision(rev)
				if err != nil {
					return errors.Wrapf(err, "failed to parse application revision %s", rev.Name)
				}
				*afPtr = af
				*specPtr = rev.Spec.Application.Spec
				return nil
			}
			if len(args) < 3 {
				latestRev := app.Status.LatestRevision
				app.Status.LatestRevision = nil
				af1, err = appParser.GenerateAppFile(ctx, app)
				app.Status.LatestRevision = latestRev
				if err != nil {
					return errors.Wrapf(err, "failed to parse current application")
				}
				spec1 = app.Spec
			}
			switch len(args) {
			case 1:
				if app.Status.LatestRevision == nil {
					cmd.Printf("Application has no revision now, current phase: %s.\n", app.Status.Phase)
					return nil
				}
				cmd.Printf("Current revision in use is %s\n", app.Status.LatestRevision.Name)
				if err = load(app.Status.LatestRevision.Name, false, &af2, &spec2); err != nil {
					return err
				}
			case 2:
				if err = load(args[1], true, &af2, &spec2); err != nil {
					return err
				}
			case 3:
				if err = load(args[1], true, &af1, &spec1); err != nil {
					return err
				}
				if err = load(args[2], true, &af2, &spec2); err != nil {
					return err
				}
			}
			diffSpec, specChanged, err := diffAppSpec(spec2, spec1)
			if err != nil {
				return err
			}
			cmd.Printf("%s%s%s\n%s\n", color.CyanString("=== Application Spec ("), diffMarker(specChanged), color.CyanString(") ==="), diffSpec)
			diffExternal, externalChanged, err := diffExternalPoliciesAndWorkflowInAppfile(af2, af1)
			if err != nil {
				return err
			}
			cmd.Printf("%s%s%s\n%s\n", color.CyanString("=== External Policies and Workflow ("), diffMarker(externalChanged), color.CyanString(") ==="), diffExternal)
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

func findRevInAppRevs(name string, revs []v1beta1.ApplicationRevision, searchPublishVersion bool) (*v1beta1.ApplicationRevision, error) {
	for _, _rev := range revs {
		rev := _rev.DeepCopy()
		if rev.Name == name {
			return rev, nil
		}
		if searchPublishVersion && oam.GetPublishVersion(rev) == name {
			return rev, nil
		}
	}
	return nil, errors.Errorf("failed to find application revision with name or PublishVersion equal to %s", name)
}

func diffMarker(changed bool) string {
	if changed {
		return color.YellowString("Modified")
	}
	return color.CyanString("No Change")
}

func diffAppSpec(a, b v1beta1.ApplicationSpec) (string, bool, error) {
	original, err := yaml.Marshal(a)
	if err != nil {
		return "", false, errors.Wrapf(err, "cannot marshal original application spec into yaml")
	}
	current, err := yaml.Marshal(b)
	if err != nil {
		return "", false, errors.Wrapf(err, "cannot marshal current application spec into yaml")
	}
	appSpecDiff := diffString(string(original), string(current))
	if appSpecDiff == "" {
		appSpecDiff = "No diff in application spec."
	}
	return appSpecDiff, string(original) != string(current), nil
}

func diffExternalPoliciesAndWorkflowInAppfile(a, b *appfile.Appfile) (string, bool, error) {
	original, err := encodeExternalPoliciesAndWorkflowInAppfile(a)
	if err != nil {
		return "", false, errors.Wrapf(err, "encode revision error")
	}
	current, err := encodeExternalPoliciesAndWorkflowInAppfile(b)
	if err != nil {
		return "", false, errors.Wrapf(err, "encode application error")
	}
	diff := diffString(original, current)
	if diff == "" {
		diff = "No diff for external policies and workflow."
	}
	return diff, original != current, nil
}

func getAppParser(c common.Args, cli client.Client) (*appfile.Parser, error) {
	cfg, err := c.GetConfig()
	if err != nil {
		return nil, err
	}
	dm, err := discoverymapper.New(cfg)
	if err != nil {
		return nil, err
	}
	pd, err := packages.NewPackageDiscover(cfg)
	if err != nil {
		return nil, err
	}
	appParser := appfile.NewApplicationParser(cli, dm, pd)
	return appParser, nil
}

func encodeExternalPoliciesAndWorkflowInAppfile(af *appfile.Appfile) (string, error) {
	var templates []string
	var policies []*v1alpha1.Policy
	for _, p := range af.ExternalPolicies {
		po := &v1alpha1.Policy{}
		if p.Object != nil {
			po = p.Object.(*v1alpha1.Policy)
		} else if err := json.Unmarshal(p.Raw, po); err != nil {
			return "", errors.Wrapf(err, "failed to decode policy %s", po.Name)
		}
		policies = append(policies, po)
	}
	if len(policies) > 1 {
		sort.Slice(policies, func(i, j int) bool {
			return policies[i].Name < policies[j].Name
		})
	}
	for _, po := range policies {
		bs, _ := yaml.Marshal(po.Properties)
		templates = append(templates, fmt.Sprintf("apiVersion: core.oam.dev/v1alpha1\nkind: Policy\nmetadata:\n  name: %s\n  namespace: %s\ntype: %s\nproperties:\n%s", po.Name, po.Namespace, po.Type, indentString(string(bs))))
	}
	if af.ExternalWorkflow != nil {
		wf := &v1alpha1.Workflow{}
		if af.ExternalWorkflow.Object != nil {
			wf = af.ExternalWorkflow.Object.(*v1alpha1.Workflow)
		} else if err := json.Unmarshal(af.ExternalWorkflow.Raw, wf); err != nil {
			return "", errors.Wrapf(err, "failed to decode workflow")
		}
		bs, _ := yaml.Marshal(wf.Steps)
		templates = append(templates, fmt.Sprintf("apiVersion: core.oam.dev/v1alpha1\nkind: Workflow\nmetadata:\n  name: %s\n  namespace: %s\nsteps:\n%s\n", wf.Name, wf.Namespace, indentString(string(bs))))
	}
	return strings.Join(templates, "---\n"), nil
}

func indentString(s string) string {
	return "  " + strings.TrimSpace(strings.ReplaceAll(s, "\n", "\n  ")) + "\n"
}
