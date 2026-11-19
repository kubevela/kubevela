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

// Package reader implements vela/velaconfig's Reader over pkg/config. It sits
// apart from the provider because pkg/config reaches back into pkg/cue/cuex,
// which imports the provider; nothing on that path imports this package.
package reader

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubevela/pkg/util/singleton"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/velaconfig"
)

// Config reads a Config through pkg/config, which also refuses one
// marked sensitive - a guard worth inheriting rather than reimplementing in the
// provider.
type Config struct {
	// Client is the cluster the Config is read from; nil is the process's.
	Client client.Client
}

// ReadConfig reads the named Config's properties, template and outputs.
func (r Config) ReadConfig(ctx context.Context, namespace, name string) (*velaconfig.ReadResult, error) {
	cli := r.Client
	if cli == nil {
		cli = singleton.KubeClient.Get()
	}
	factory := config.NewConfigFactory(cli)

	// ReadConfig first, and deliberately: it is the call that returns
	// ErrSensitiveConfig. GetConfig below merely blanks a sensitive Config's
	// properties and secret data, so on its own it would let a sensitive
	// Config's template and output references through.
	props, err := factory.ReadConfig(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	// The second read is for the template and the output references, which
	// ReadConfig does not carry. withStatus is false - distribution status
	// describes where a Config has been replicated to, which is an operational
	// concern rather than data a workload should be shaping itself around.
	cfg, err := factory.GetConfig(ctx, namespace, name, false)
	if err != nil {
		return nil, err
	}

	result := &velaconfig.ReadResult{
		Properties: props,
		Template: velaconfig.TemplateRef{
			Name:      cfg.Template.Name,
			Namespace: cfg.Template.Namespace,
		},
		// A template's `output` is rendered into the Config's own Secret, whose
		// identity is the Config's. Naming it costs nothing - no render, no
		// lookup - and it makes the one object every Config has addressable
		// alongside the rest.
		Output: velaconfig.ObjectRef{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       name,
			Namespace:  namespace,
		},
		Outputs: map[string]velaconfig.ObjectRef{},
	}

	// A template with no `outputs:` block skips the render below entirely, which
	// is the common case and the one worth keeping cheap.
	if len(cfg.ObjectReferences) == 0 {
		return result, nil
	}

	names := renderedOutputNames(ctx, factory, cfg, namespace, name, props)
	for _, ref := range cfg.ObjectReferences {
		// The stored reference is the authority on identity - it names what was
		// actually applied. The render only supplies the label.
		key, ok := names[objectIdentity(ref.APIVersion, ref.Kind, ref.Namespace, ref.Name)]
		if !ok {
			key = fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
		}
		result.Outputs[key] = velaconfig.ObjectRef{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			Namespace:  ref.Namespace,
		}
	}
	return result, nil
}

// renderedOutputNames recovers the name a template gave each of its outputs.
//
// Nothing stored on a Config carries them: pkg/config keeps OutputObjects keyed
// by name in memory but serialises only a flat list of references, so the names
// exist solely in the template. Re-rendering is the one way to get them back.
//
// That is not free - ParseConfig recompiles the template, which resolves every
// provider call in it including a `validation:` block, and some validators reach
// the network. The source's storageTTL is what makes it tolerable: this runs on
// a cache miss, not per reconcile.
//
// A failure here is not fatal. The names are a convenience; identity comes from
// the stored references either way, so the caller falls back to Kind/name keys
// rather than failing a read that would otherwise have succeeded.
func renderedOutputNames(ctx context.Context, factory config.Factory, cfg *config.Config,
	namespace, name string, props map[string]interface{},
) map[string]string {
	rendered, err := factory.ParseConfig(ctx,
		config.NamespacedName{Name: cfg.Template.Name, Namespace: cfg.Template.Namespace},
		config.Metadata{
			NamespacedName: config.NamespacedName{Name: name, Namespace: namespace},
			Properties:     props,
		})
	if err != nil {
		klog.V(2).InfoS("Cannot re-render config template to name its outputs; falling back to Kind/name",
			"config", name, "namespace", namespace, "err", err)
		return nil
	}

	names := make(map[string]string, len(rendered.OutputObjects))
	for label, obj := range rendered.OutputObjects {
		names[objectIdentity(obj.GetAPIVersion(), obj.GetKind(), obj.GetNamespace(), obj.GetName())] = label
	}
	return names
}

func objectIdentity(apiVersion, kind, namespace, name string) string {
	return strings.Join([]string{apiVersion, kind, namespace, name}, "|")
}
