/*
Copyright 2022 The KubeVela Authors.

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

package registry

import (
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Registry interface {
	// get image config
	Config(image string) (*v1.ConfigFile, error)
}

type registry struct {
	opts options
}

func NewRegistry(opts ...Option) *registry {
	return &registry{
		opts: makeOptions(opts...),
	}
}

func (r *registry) Config(image string) (*v1.ConfigFile, error) {
	img, _, err := r.getImage(image)
	if err != nil {
		return nil, err
	}

	configFile, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	return configFile, nil
}

func (r *registry) getImage(reference string) (v1.Image, name.Reference, error) {
	ref, err := name.ParseReference(reference, r.opts.name...)
	if err != nil {
		return nil, nil, err
	}

	img, err := remote.Image(ref, r.opts.remote...)
	if err != nil {
		return nil, nil, err
	}

	return img, ref, nil
}
