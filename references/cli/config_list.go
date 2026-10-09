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

package cli

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gosuri/uitable"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

// Values of the --config-mode flag carried by the two list commands. The empty
// default shows both backends.
const (
	configModeLegacy = "legacy"
	configModeCRD    = "crd"

	configModeFlagUsage = "Show only one backend: legacy (ConfigMaps/Secrets) or crd (ConfigTemplate/Config CRs). Default shows both."
)

// Values of the SOURCE column.
const (
	sourceCRD            = "crd"
	sourceLegacy         = "legacy"
	sourceLegacyShadowed = "legacy (shadowed)"
)

// validateConfigMode rejects anything but legacy, crd or the empty default. It
// runs before any API call so a typo fails fast.
func validateConfigMode(mode string) error {
	switch mode {
	case "", configModeLegacy, configModeCRD:
		return nil
	}
	return fmt.Errorf("invalid --config-mode %q: use legacy or crd", mode)
}

// nsNameLess orders rows by namespace, then name, so list output is stable.
func nsNameLess(ns1, name1, ns2, name2 string) bool {
	if ns1 != ns2 {
		return ns1 < ns2
	}
	return name1 < name2
}

// templateRow is one line of `config-template list`, whichever backend it came from.
type templateRow struct {
	namespace, name, alias, scope string
	sensitive                     bool
	source                        string
	created                       time.Time
}

func sortTemplateRows(rows []templateRow) {
	sort.Slice(rows, func(i, j int) bool {
		return nsNameLess(rows[i].namespace, rows[i].name, rows[j].namespace, rows[j].name)
	})
}

// listTemplateRows returns CR rows first, then legacy rows, each group sorted by
// namespace then name. In merged mode a legacy template whose name a CR also
// holds in the same namespace is marked shadowed. ns "" means all namespaces.
// A cluster without the ConfigTemplate CRD contributes no CR rows and no error.
func listTemplateRows(ctx context.Context, cli client.Client, ns, mode string) ([]templateRow, error) {
	var rows []templateRow
	crNames := map[config.NamespacedName]bool{}
	if mode != configModeLegacy {
		items, err := listConfigTemplateCRDs(ctx, cli, ns)
		if err != nil {
			return nil, err
		}
		for _, t := range items {
			crNames[config.NamespacedName{Namespace: t.Namespace, Name: t.Name}] = true
			rows = append(rows, templateRow{
				namespace: t.Namespace, name: t.Name, alias: t.Spec.Alias, scope: string(t.Spec.Scope),
				sensitive: t.Spec.Sensitive, source: sourceCRD, created: t.CreationTimestamp.Time,
			})
		}
		sortTemplateRows(rows)
	}
	if mode != configModeCRD {
		legacy, err := config.NewConfigFactory(cli).ListTemplates(ctx, ns, "")
		if err != nil {
			return nil, err
		}
		var legacyRows []templateRow
		for _, t := range legacy {
			source := sourceLegacy
			if crNames[config.NamespacedName{Namespace: t.Namespace, Name: t.Name}] {
				source = sourceLegacyShadowed
			}
			legacyRows = append(legacyRows, templateRow{
				namespace: t.Namespace, name: t.Name, alias: t.Alias, scope: t.Scope,
				sensitive: t.Sensitive, source: source, created: t.CreateTime,
			})
		}
		sortTemplateRows(legacyRows)
		rows = append(rows, legacyRows...)
	}
	return rows, nil
}

// configRow is one line of `config list`. Legacy rows have no phase: nothing
// reconciles a plain Secret.
type configRow struct {
	namespace, name, alias, source, phase, distribution, template, description string
	created                                                                    time.Time
}

func sortConfigRows(rows []configRow) {
	sort.Slice(rows, func(i, j int) bool {
		return nsNameLess(rows[i].namespace, rows[i].name, rows[j].namespace, rows[j].name)
	})
}

// listConfigRows returns CR rows first, then legacy rows, each group sorted by
// namespace then name. template filters both backends by template name. No
// shadow marker is needed: a Config CR materialises a Secret of the same name
// and the legacy lister already skips CR-owned Secrets. ns "" means all
// namespaces. A cluster without the Config CRD contributes no CR rows and no error.
func listConfigRows(ctx context.Context, cli client.Client, ns, template, mode string) ([]configRow, error) {
	inf := config.NewConfigFactory(cli)
	var rows []configRow
	if mode != configModeLegacy {
		items, err := listConfigCRDs(ctx, cli, ns, template)
		if err != nil {
			return nil, err
		}
		for _, c := range items {
			tmplRef := ""
			if c.Spec.TemplateRef != nil {
				tmplNs := c.Spec.TemplateRef.Namespace
				if tmplNs == "" {
					tmplNs = types.DefaultKubeVelaNS
				}
				tmplRef = fmt.Sprintf("%s/%s", tmplNs, c.Spec.TemplateRef.Name)
			}
			rows = append(rows, configRow{
				namespace: c.Namespace, name: c.Name, alias: c.Spec.Alias, source: sourceCRD, phase: string(c.Status.Phase),
				distribution: distributionColumn(ctx, inf, c.Name, c.Namespace), template: tmplRef,
				created: c.CreationTimestamp.Time, description: c.Spec.Description,
			})
		}
		sortConfigRows(rows)
	}
	if mode != configModeCRD {
		legacy, err := inf.ListConfigs(ctx, ns, template, "", true)
		if err != nil {
			return nil, err
		}
		var legacyRows []configRow
		for _, c := range legacy {
			legacyRows = append(legacyRows, configRow{
				namespace: c.Namespace, name: c.Name, alias: c.Alias, source: sourceLegacy,
				distribution: formatDistributionTargets(c.Targets), template: fmt.Sprintf("%s/%s", c.Template.Namespace, c.Template.Name),
				created: c.CreateTime, description: c.Description,
			})
		}
		sortConfigRows(legacyRows)
		rows = append(rows, legacyRows...)
	}
	return rows, nil
}

// writeTable prints the table followed by a blank line.
func writeTable(streams util.IOStreams, table *uitable.Table) error {
	if _, err := streams.Out.Write(table.Bytes()); err != nil {
		return err
	}
	_, err := streams.Out.Write([]byte("\n"))
	return err
}
