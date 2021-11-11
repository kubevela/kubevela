/*
 Copyright 2021. The KubeVela Authors.

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

package time

import (
	"time"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "time"
)

type provider struct {
}

func (h *provider) Timestamp(ctx wfContext.Context, v *value.Value, act types.Action) error {
	date, err := v.GetString("date")
	if err != nil {
		return err
	}
	layout, err := v.GetString("layout")
	if err != nil {
		return err
	}
	if layout == "" {
		layout = time.RFC3339
	}
	t, err := time.Parse(layout, date)
	if err != nil {
		return err
	}
	return v.FillObject(t.Unix(), "timestamp")
}

func (h *provider) Date(ctx wfContext.Context, v *value.Value, act types.Action) error {
	timestamp, err := v.GetInt64("timestamp")
	if err != nil {
		return err
	}
	layout, err := v.GetString("layout")
	if err != nil {
		return err
	}
	locationName, err := v.GetString("location")
	if err != nil {
		return err
	}

	if layout == "" {
		layout = time.RFC3339
	}

	location, err := time.LoadLocation(locationName)
	if err != nil {
		return err
	}
	t := time.Unix(timestamp, 0)
	t.In(location)
	return v.FillObject(t.Format(layout), "date")
}

// Install register handlers to provider discover.
func Install(p providers.Providers) {
	prd := &provider{}
	p.Register(ProviderName, map[string]providers.Handler{
		"timestamp": prd.Timestamp,
		"date":      prd.Date,
	})
}
