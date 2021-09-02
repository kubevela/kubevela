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

package oam

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "oam"
)

// Parser parse types.ComponentManifest from application's component.
type Parser func(ctx context.Context, comp common.ApplicationComponent) (*types.ComponentManifest, error)

type provider struct {
	parse Parser
}

// Parse component manifest from value.
func (p *provider) Parse(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	compSettings, err := v.LookupValue("settings")
	if err != nil {
		return err
	}
	var comp common.ApplicationComponent
	if err := compSettings.UnmarshalTo(&comp); err != nil {
		return err
	}
	manifest, err := p.parse(context.Background(), comp)
	if err != nil {
		return errors.WithMessagef(err, "render component(%s)", comp.Name)
	}

	raw, err := json.Marshal(manifest)
	if err != nil {
		return errors.WithMessagef(err, "marshal manifest(component=%s)", comp.Name)
	}
	return v.FillRaw(string(raw), "value")
}

// Install register handlers to provider discover.
func Install(p providers.Providers, parser Parser) {
	prd := &provider{
		parse: parser,
	}
	p.Register(ProviderName, map[string]providers.Handler{
		"parse": prd.Parse,
	})
}
